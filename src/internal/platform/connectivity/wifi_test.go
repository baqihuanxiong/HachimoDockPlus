package connectivity

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fakeWPA(t *testing.T, respond func(string) string) (*Service, *[]string, *sync.Mutex) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wlan0")
	c, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	commands := []string{}
	mu := &sync.Mutex{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		b := make([]byte, 8192)
		for {
			n, addr, e := c.ReadFromUnix(b)
			if e != nil {
				return
			}
			mu.Lock()
			cmd := string(b[:n])
			commands = append(commands, cmd)
			out := respond(cmd)
			mu.Unlock()
			if out != "" {
				_, _ = c.WriteToUnix([]byte(out), addr)
			}
		}
	}()
	t.Cleanup(func() { c.Close(); <-done })
	return &Service{Socket: path}, &commands, mu
}
func TestScanParsing(t *testing.T) {
	n := parseNetworks("bssid / frequency / signal level / flags / ssid\na\t2412\t-62\t[WPA2-PSK-CCMP][ESS]\t\\xe4\\xb8\\xad\\xe6\\x96\\x87\nb\t2412\t-45\t[WPA2-PSK-CCMP][ESS]\tGuest\nc\t2412\t-70\t[ESS]\tGuest\nd\t2412\t-80\t[SAE][ESS]\tSecure\ne\t2412\t-81\t[ESS]\tOpen", "network id / ssid / bssid / flags\n0\t\\xe4\\xb8\\xad\\xe6\\x96\\x87\tany\t[CURRENT]")
	if len(n) != 5 || n[0].SSID != "中文" || !n[0].Current || n[1].Signal != -45 || n[2].Security != "开放" || n[3].Security != "不支持" || n[4].Security != "开放" {
		t.Fatal(n)
	}
}
func TestJoinSavesOnlyAfterAssociationAndDoesNotExposePassword(t *testing.T) {
	selected := false
	s, commands, mu := fakeWPA(t, func(cmd string) string {
		switch cmd {
		case "ADD_NETWORK":
			return "2"
		case "STATUS":
			if selected {
				return "id=2\nwpa_state=COMPLETED\nip_address=10.0.0.2"
			}
			return "id=0\nwpa_state=COMPLETED"
		case "SELECT_NETWORK 2":
			selected = true
		}
		return "OK"
	})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := s.JoinWiFi(ctx, Join{SSID: `Cafe";$(touch /tmp/no)`, Password: "secret123", Security: "WPA-PSK"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	all := strings.Join(*commands, "\n")
	if strings.Contains(all, "secret123") || strings.Contains(all, "$(touch") {
		t.Fatal("unencoded credential or SSID")
	}
	if !strings.Contains(all, "SET_NETWORK 2 psk ") || !strings.HasSuffix(all, "SAVE_CONFIG") {
		t.Fatal(all)
	}
}
func TestFailedJoinRollsBackWithoutSaving(t *testing.T) {
	selected := false
	s, commands, mu := fakeWPA(t, func(cmd string) string {
		switch cmd {
		case "ADD_NETWORK":
			return "2"
		case "STATUS":
			if !selected {
				return "id=0\nwpa_state=COMPLETED"
			}
			return "wpa_state=4WAY_HANDSHAKE"
		case "SELECT_NETWORK 2":
			selected = true
		}
		return "OK"
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := s.JoinWiFi(ctx, Join{SSID: "wrong", Password: "badpass12", Security: "WPA-PSK"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	all := strings.Join(*commands, "\n")
	if strings.Contains(all, "SAVE_CONFIG") || !strings.Contains(all, "SELECT_NETWORK 0") || !strings.Contains(all, "REMOVE_NETWORK 2") {
		t.Fatal(all)
	}
}
func TestValidationAndSocketCancellation(t *testing.T) {
	s, commands, mu := fakeWPA(t, func(string) string { return "" })
	for _, j := range []Join{{SSID: "a", Password: "short", Security: "WPA-PSK"}, {SSID: "a", Security: "不支持"}, {SSID: strings.Repeat("中", 12), Security: "开放"}} {
		if s.JoinWiFi(context.Background(), j) == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if s.ConnectSaved(context.Background(), "0\nSAVE_CONFIG") == nil || s.Bluetooth(context.Background(), "connect", "--help") == nil {
		t.Fatal("invalid identifier accepted")
	}
	mu.Lock()
	n := len(*commands)
	mu.Unlock()
	if n != 0 {
		t.Fatal("validation sent commands")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := s.request(ctx, "STATUS")
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatal(err)
	}
}
