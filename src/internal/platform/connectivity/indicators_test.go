package connectivity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSignalStrength(t *testing.T) {
	for _, tc := range []struct{ rssi, bars int }{{-90, 1}, {-76, 1}, {-75, 2}, {-66, 2}, {-65, 3}, {-56, 3}, {-55, 4}, {-30, 4}} {
		if got := signalStrength(tc.rssi); got != tc.bars {
			t.Fatal(tc, got)
		}
	}
}
func TestBluetoothTypes(t *testing.T) {
	for _, tc := range []struct {
		info string
		kind int
	}{
		{"Icon: audio-headphones", BluetoothHeadphones}, {"Icon: audio-card", BluetoothSpeaker}, {"Icon: input-keyboard", BluetoothKeyboard}, {"Icon: input-mouse", BluetoothMouse}, {"Icon: input-gaming", BluetoothGamepad}, {"Icon: phone", BluetoothPhone}, {"Icon: computer", BluetoothComputer}, {"Icon: watch", BluetoothWatch}, {"Icon: camera-photo", BluetoothCamera},
		{"Class: 0x240404", BluetoothHeadphones}, {"Class: 0x002540", BluetoothKeyboard}, {"Class: 0x002580", BluetoothMouse}, {"Class: 0x000104", BluetoothComputer}, {"Class: invalid", BluetoothGeneric}, {"", BluetoothGeneric},
	} {
		if got := bluetoothKind(tc.info); got != tc.kind {
			t.Fatal(tc, got)
		}
	}
}
func TestReadOnlyIndicators(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
 show) printf 'Controller AA:BB:CC:DD:EE:FF\nPowered: yes\n';;
 devices) printf 'Device 11:22:33:44:55:66 Headphones\n';;
 info) printf 'Connected: yes\nIcon: audio-headphones\n';;
 *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "bluetoothctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	s, commands, mu := fakeWPA(t, func(cmd string) string {
		switch cmd {
		case "STATUS":
			return "wpa_state=COMPLETED"
		case "SIGNAL_POLL":
			return "RSSI=-62"
		}
		return "FAIL"
	})
	st := s.Indicators(context.Background())
	if !st.WiFiConnected || st.WiFiStrength != 3 || !st.BluetoothConnected || st.BluetoothKind != BluetoothHeadphones {
		t.Fatal(st)
	}
	mu.Lock()
	recorded := append([]string(nil), (*commands)...)
	mu.Unlock()
	if len(recorded) != 2 {
		t.Fatal(recorded)
	}
	// Even if BlueZ lists a stale device, only live Connected: yes may show an icon.
	script = `#!/bin/sh
case "$1" in
 show) printf 'Controller AA:BB:CC:DD:EE:FF\nPowered: yes\n';;
 devices) echo 'Device 11:22:33:44:55:66 Headphones';;
 info) printf 'Connected: no\nIcon: audio-headphones\n';;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "bluetoothctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	st = s.Indicators(context.Background())
	if st.BluetoothConnected || st.BluetoothKind != BluetoothNone {
		t.Fatal("stale device shown", st)
	}
}
