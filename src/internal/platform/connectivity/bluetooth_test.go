package connectivity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBluetoothQueriesReturnWithoutCLITimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bluetoothctl")
	script := `#!/bin/sh
# BlueZ --timeout delays even synchronous show/devices until timeout expiry.
case "$1" in
 show) printf 'Controller AA:BB:CC:DD:EE:FF\n\tPowered: yes\n';;
 devices) echo 'Device 11:22:33:44:55:66 Test speaker';;
 info) printf 'Device 11:22:33:44:55:66\n\tPaired: yes\n\tConnected: yes\n';;
 power) echo 'Failed to set power';;
 *) exit 1;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	st := State{}
	New().readBluetooth(context.Background(), &st)
	if !st.BluetoothReady || !st.BluetoothOn || st.BluetoothError != "" || len(st.Devices) != 1 || !st.Devices[0].Paired || !st.Devices[0].Connected {
		t.Fatal(st)
	}
	if New().Bluetooth(context.Background(), "off", "") == nil {
		t.Fatal("BlueZ exit-zero failure was accepted")
	}
}
