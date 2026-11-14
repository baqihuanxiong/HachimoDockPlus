// Package connectivity adapts the board's existing wpa_supplicant and BlueZ services.
package connectivity

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Network struct {
	ID, SSID, Security string
	Signal             int
	Saved, Current     bool
}
type Device struct {
	Address, Name              string
	Paired, Connected, Trusted bool
}
type State struct {
	WiFiStatus, SSID, IP, WiFiError string
	Networks                        []Network
	BluetoothReady, BluetoothOn     bool
	BluetoothError                  string
	Devices                         []Device
}
type Join struct{ SSID, Password, Security string }
type Service struct{ Socket string }

func New() *Service { return &Service{Socket: "/var/run/wpa_supplicant/wlan0"} }

// Use the control socket directly: credentials never appear in process arguments or errors.
func (s *Service) request(ctx context.Context, command string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "dock-wpa-")
	if err != nil {
		return "", errors.New("无法创建 Wi-Fi 控制连接")
	}
	defer os.RemoveAll(dir)
	c, err := net.DialUnix("unixgram", &net.UnixAddr{Name: filepath.Join(dir, "ctrl"), Net: "unixgram"}, &net.UnixAddr{Name: s.Socket, Net: "unixgram"})
	if err != nil {
		return "", errors.New("Wi-Fi 服务未就绪")
	}
	defer c.Close()
	deadline := time.Now().Add(3 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = c.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	if _, err = c.Write([]byte(command)); err != nil {
		return "", errors.New("Wi-Fi 控制请求失败")
	}
	b := make([]byte, 65536)
	n, err := c.Read(b)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("Wi-Fi 服务响应超时")
	}
	out := strings.TrimSpace(string(b[:n]))
	if strings.HasPrefix(out, "FAIL") || strings.HasPrefix(out, "UNKNOWN") {
		return "", errors.New("Wi-Fi 操作失败")
	}
	return out, nil
}
func fields(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			m[k] = v
		}
	}
	return m
}
func decodeSSID(v string) string { // wpa control output escapes non-ASCII bytes as \xNN.
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' && i+1 < len(v) {
			if v[i+1] == 'x' && i+3 < len(v) {
				if n, e := strconv.ParseUint(v[i+2:i+4], 16, 8); e == nil {
					b.WriteByte(byte(n))
					i += 3
					continue
				}
			}
			if v[i+1] == '\\' || v[i+1] == '"' {
				i++
				b.WriteByte(v[i])
				continue
			}
		}
		b.WriteByte(v[i])
	}
	return b.String()
}
func parseNetworks(scan, saved string) []Network {
	networks := []Network{}
	seen := map[string]bool{}
	savedSSID := map[string]bool{}
	for _, line := range strings.Split(saved, "\n")[1:] {
		p := strings.SplitN(line, "\t", 4)
		if len(p) < 4 {
			continue
		}
		networks = append(networks, Network{ID: p[0], SSID: decodeSSID(p[1]), Saved: true, Current: strings.Contains(p[3], "CURRENT"), Security: "已保存"})
		savedSSID[decodeSSID(p[1])] = true
	}
	for _, line := range strings.Split(scan, "\n")[1:] {
		p := strings.SplitN(line, "\t", 5)
		if len(p) < 5 || p[4] == "" {
			continue
		}
		ssid := decodeSSID(p[4])
		signal, _ := strconv.Atoi(p[2])
		security := "开放"
		switch {
		case strings.Contains(p[3], "PSK"):
			security = "WPA-PSK"
		case strings.Contains(p[3], "SAE"), strings.Contains(p[3], "EAP"), strings.Contains(p[3], "WEP"):
			security = "不支持"
		}
		if savedSSID[ssid] || seen[ssid+"\x00"+security] {
			for i := range networks {
				if networks[i].SSID == ssid && (networks[i].Saved || networks[i].Security == security) && (networks[i].Signal == 0 || signal > networks[i].Signal) {
					networks[i].Signal = signal
				}
			}
			continue
		}
		seen[ssid+"\x00"+security] = true
		networks = append(networks, Network{SSID: ssid, Signal: signal, Security: security})
	}
	sort.SliceStable(networks, func(i, j int) bool {
		if networks[i].Current != networks[j].Current {
			return networks[i].Current
		}
		if networks[i].Saved != networks[j].Saved {
			return networks[i].Saved
		}
		return networks[i].Signal > networks[j].Signal
	})
	return networks
}
func (s *Service) Read(ctx context.Context) State {
	var st State
	status, err := s.request(ctx, "STATUS")
	if err != nil {
		st.WiFiError = err.Error()
	} else {
		m := fields(status)
		st.WiFiStatus = m["wpa_state"]
		st.SSID = decodeSSID(m["ssid"])
		st.IP = m["ip_address"]
		saved, e := s.request(ctx, "LIST_NETWORKS")
		if e != nil {
			st.WiFiError = e.Error()
		} else {
			scan, e := s.request(ctx, "SCAN_RESULTS")
			if e != nil {
				st.WiFiError = e.Error()
			}
			st.Networks = parseNetworks(scan, saved)
		}
	}
	s.readBluetooth(ctx, &st)
	return st
}
func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func (s *Service) ScanWiFi(ctx context.Context) error {
	if _, err := s.request(ctx, "SCAN"); err != nil {
		return err
	}
	return wait(ctx, 4*time.Second)
}
func validID(id string) bool { n, e := strconv.Atoi(id); return e == nil && n >= 0 && n < 100000 }
func (s *Service) save(ctx context.Context) error {
	if _, err := s.request(ctx, "SET update_config 1"); err != nil {
		return err
	}
	if _, err := s.request(ctx, "SAVE_CONFIG"); err != nil {
		return errors.New("保存 Wi-Fi 配置失败，请检查 update_config 和文件权限")
	}
	return nil
}
func (s *Service) DisconnectWiFi(ctx context.Context) error {
	_, err := s.request(ctx, "DISCONNECT")
	return err
}
func (s *Service) ForgetWiFi(ctx context.Context, id string) error {
	if !validID(id) {
		return errors.New("无效的网络编号")
	}
	if _, err := s.request(ctx, "REMOVE_NETWORK "+id); err != nil {
		return err
	}
	return s.save(ctx)
}
func (s *Service) ConnectSaved(ctx context.Context, id string) error {
	if !validID(id) {
		return errors.New("无效的网络编号")
	}
	return s.connect(ctx, id)
}
func (s *Service) JoinWiFi(ctx context.Context, j Join) error {
	if len(j.SSID) == 0 || len(j.SSID) > 32 {
		return errors.New("SSID 必须为 1～32 字节")
	}
	var psk string
	switch j.Security {
	case "开放":
		if j.Password != "" {
			return errors.New("开放网络无需密码")
		}
	case "WPA-PSK":
		if len(j.Password) == 64 {
			if _, e := hex.DecodeString(j.Password); e != nil {
				return errors.New("64 位密码必须为十六进制 PSK")
			}
			psk = j.Password
		} else {
			if len(j.Password) < 8 || len(j.Password) > 63 {
				return errors.New("Wi-Fi 密码须为 8～63 字节")
			}
			key, e := pbkdf2.Key(sha1.New, j.Password, []byte(j.SSID), 4096, 32)
			if e != nil {
				return errors.New("无法生成 Wi-Fi 密钥")
			}
			psk = hex.EncodeToString(key)
		}
	default:
		return errors.New("目前支持开放网络和 WPA/WPA2 Personal")
	}
	id, err := s.request(ctx, "ADD_NETWORK")
	if err != nil {
		return err
	}
	if !validID(id) {
		return errors.New("Wi-Fi 返回无效网络编号")
	}
	keep := false
	defer func() {
		if !keep {
			clean, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _ = s.request(clean, "REMOVE_NETWORK "+id)
		}
	}()
	commands := []string{"SET_NETWORK " + id + " ssid " + hex.EncodeToString([]byte(j.SSID))}
	if psk == "" {
		commands = append(commands, "SET_NETWORK "+id+" key_mgmt NONE")
	} else {
		commands = append(commands, "SET_NETWORK "+id+" key_mgmt WPA-PSK", "SET_NETWORK "+id+" psk "+psk)
	}
	for _, cmd := range commands {
		if _, err = s.request(ctx, cmd); err != nil {
			return err
		}
	}
	err = s.connect(ctx, id)
	keep = err == nil
	return err
}
func (s *Service) connect(ctx context.Context, id string) error {
	before, err := s.request(ctx, "STATUS")
	if err != nil {
		return err
	}
	previous := fields(before)["id"]
	saved, err := s.request(ctx, "LIST_NETWORKS")
	if err != nil {
		return err
	}
	restore := func(clean context.Context, chosen string) error {
		var result error
		for _, line := range strings.Split(saved, "\n")[1:] {
			p := strings.SplitN(line, "\t", 4)
			if len(p) != 4 || !validID(p[0]) || p[0] == chosen {
				continue
			}
			command := "ENABLE_NETWORK "
			if strings.Contains(p[3], "DISABLED") {
				command = "DISABLE_NETWORK "
			}
			if _, err := s.request(clean, command+p[0]); err != nil && result == nil {
				result = err
			}
		}
		return result
	}
	// SELECT_NETWORK disables other entries; rollback on failure/cancel, and re-enable
	// alternatives before saving while preserving deliberately disabled entries.
	success := false
	defer func() {
		if !success {
			clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if validID(previous) && previous != id {
				_, _ = s.request(clean, "SELECT_NETWORK "+previous)
			}
			_ = restore(clean, previous)
			if !validID(previous) {
				_, _ = s.request(clean, "DISCONNECT")
			}
		}
	}()
	if _, err := s.request(ctx, "SELECT_NETWORK "+id); err != nil {
		return err
	}
	for {
		if err := wait(ctx, time.Second); err != nil {
			return err
		}
		out, err := s.request(ctx, "STATUS")
		if err != nil {
			return err
		}
		m := fields(out)
		if m["id"] == id && m["wpa_state"] == "COMPLETED" && m["ip_address"] != "" {
			break
		}
	}
	if err := restore(ctx, id); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.save(ctx); err != nil {
		return err
	}
	success = true
	return nil
}
func (s State) Summary() string {
	return fmt.Sprintf("Wi-Fi: %s  %s\nIP: %s\n网络: %d\n蓝牙: %t，设备: %d", s.WiFiStatus, s.SSID, s.IP, len(s.Networks), s.BluetoothOn, len(s.Devices))
}
