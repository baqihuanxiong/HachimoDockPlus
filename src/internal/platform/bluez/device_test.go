package bluez

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDeviceCallUpstreamAndRockchip(t *testing.T) {
	for _, method := range []string{"Connect", "Disconnect"} {
		for _, vendor := range []bool{false, true} {
			t.Run(method+map[bool]string{false: "/upstream", true: "/rockchip"}[vendor], func(t *testing.T) {
				var calls [][]string
				execute := func(_ context.Context, args ...string) ([]byte, error) {
					calls = append(calls, append([]string(nil), args...))
					if vendor && len(calls) == 1 {
						return []byte("Error org.freedesktop.DBus.Error.UnknownMethod: signature missing"), errors.New("exit 1")
					}
					return []byte("method return"), nil
				}
				if err := deviceCall(context.Background(), "fc:e8:06:30:c7:f0", method, execute); err != nil {
					t.Fatal(err)
				}
				want := []string{"--system", "--dest=org.bluez", "--type=method_call", "--print-reply", "--reply-timeout=24000", "/org/bluez/hci0/dev_FC_E8_06_30_C7_F0", "org.bluez.Device1." + method}
				if !reflect.DeepEqual(calls[0], want) {
					t.Fatal(calls[0])
				}
				if vendor {
					want = append(want, "string:auto")
					if len(calls) != 2 || !reflect.DeepEqual(calls[1], want) {
						t.Fatal(calls)
					}
				} else if len(calls) != 1 {
					t.Fatal(calls)
				}
			})
		}
	}
}

func TestAudioProfileAndConnectionErrors(t *testing.T) {
	for _, reply := range []string{"", "org.bluez.Error.AlreadyConnected", "org.bluez.Error.NotReady", "org.freedesktop.DBus.Error.UnknownMethod"} {
		t.Run(reply, func(t *testing.T) {
			calls := 0
			err := deviceCall(context.Background(), "FC:E8:06:30:C7:F0", "ConnectProfile", func(_ context.Context, args ...string) ([]byte, error) {
				calls++
				if args[len(args)-1] != "string:"+audioSink {
					t.Fatal(args)
				}
				if reply != "" {
					return []byte(reply), errors.New("exit 1")
				}
				return nil, nil
			}, "string:"+audioSink)
			wantError := reply != "" && !strings.Contains(reply, "AlreadyConnected")
			if (err != nil) != wantError || calls != 1 {
				t.Fatal(err, calls)
			}
		})
	}
	called := false
	err := deviceCall(context.Background(), "invalid", "Connect", func(context.Context, ...string) ([]byte, error) {
		called = true
		return nil, nil
	})
	if err == nil || called {
		t.Fatal("invalid address reached D-Bus", err, called)
	}
}
