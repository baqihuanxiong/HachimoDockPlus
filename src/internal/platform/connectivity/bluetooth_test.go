package connectivity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestBluetoothScanRetainsNamesAndAudioTypeAfterBlueZExpiresDevices(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
 show) printf 'Controller AA:BB:CC:DD:EE:FF\n\tPowered: yes\n';;
 devices) exit 0;;
 '') read -r command; printf 'Discovery started\n[NEW] Device 11:22:33:44:55:66 11-22-33-44-55-66\n[CHG] Device 11:22:33:44:55:66 Name: \\xE8\\x80\\xB3\\xE6\\x9C\\xBA\n[CHG] Device 11:22:33:44:55:66 Icon: audio-headphones\n[CHG] Device 11:22:33:44:55:66 Alias: 我的耳机\n[NEW] Device AA:22:33:44:55:66 AA2233445566\n';;
 power|remove) exit 0;;
 *) exit 1;;
esac
`
	os.WriteFile(filepath.Join(dir, "bluetoothctl"), []byte(script), 0700)
	t.Setenv("PATH", dir)
	service := New()
	if err := service.Bluetooth(context.Background(), "scan", ""); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		var st State
		service.readBluetooth(context.Background(), &st)
		if len(st.Devices) != 2 || st.Devices[0].Name != "我的耳机" || !st.Devices[0].Audio || st.Devices[0].Kind != BluetoothHeadphones || st.Devices[0].Available || st.Devices[0].Connected {
			t.Fatal(st)
		}
		if st.Devices[1].Name != "未命名设备" {
			t.Fatal("hexadecimal placeholder displayed as name", st.Devices[1])
		}
	}
	if err := service.Bluetooth(context.Background(), "remove", "11:22:33:44:55:66"); err != nil {
		t.Fatal(err)
	}
	var st State
	service.readBluetooth(context.Background(), &st)
	if len(st.Devices) != 1 {
		t.Fatal("removed device retained", st)
	}
	if err := service.Bluetooth(context.Background(), "off", ""); err != nil {
		t.Fatal(err)
	}
	if len(service.btDevices) != 0 {
		t.Fatal("power off retained old scan")
	}
}

func TestBluetoothInfoUsesAliasAndRecognizesSinkUUID(t *testing.T) {
	d := deviceInfo(Device{}, "Name: Factory\nAlias: My headphones\nUUID: Audio Sink (0000110b-0000-1000-8000-00805f9b34fb)\nPaired: yes\nConnected: no\n")
	if d.Name != "My headphones" || !d.Audio || !d.Paired || d.Connected {
		t.Fatal(d)
	}
}

func TestSavedBluetoothRecordDoesNotCountAsDiscovery(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
 show) printf 'Controller AA:BB:CC:DD:EE:FF\n\tPowered: yes\n';;
 devices) printf 'Device FC:E8:06:30:C7:F0 EDIFIER S1000MKII\nDevice 11:22:33:44:55:66 Old neighbour\n';;
 info)
  if [ "$2" = FC:E8:06:30:C7:F0 ]; then
   printf 'Name: EDIFIER S1000MKII\nPaired: no\nTrusted: yes\nConnected: no\nRSSI: -40\n'
  else
   printf 'Name: Old neighbour\nPaired: no\nTrusted: no\nConnected: no\nRSSI: -40\n'
  fi;;
 *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "bluetoothctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	service := New()
	for range 2 {
		var st State
		service.readBluetooth(context.Background(), &st)
		if len(st.Devices) != 1 || !st.Devices[0].Trusted || st.Devices[0].Available || st.Devices[0].Connected {
			t.Fatal("cached objects/RSSI were presented as live discovery", st)
		}
	}
}

func TestBluetoothPresenceRequiresRadioEventsAndExpires(t *testing.T) {
	const address = "FC:E8:06:30:C7:F0"
	devices := map[string]Device{}
	updateScanDevices(devices, "[NEW] Device "+address+" EDIFIER S1000MKII", false)
	updateScanDevices(devices, "[CHG] Device "+address+" Trusted: yes", false)
	updateScanDevices(devices, "[CHG] Device "+address+" Alias: Saved speaker", true)
	if devices[address].Available || !devices[address].lastSeen.IsZero() {
		t.Fatal("enumeration/metadata established presence", devices[address])
	}
	updateScanDevices(devices, "[CHG] Device "+address+" RSSI: -45", true)
	d := devices[address]
	if !d.Available || d.lastSeen.IsZero() {
		t.Fatal("live advertisement not retained", d)
	}
	present := map[string]bool{address: true}
	refreshBluetoothPresence(devices, present, d.lastSeen.Add(bluetoothPresenceTTL+time.Second))
	if devices[address].Available || !devices[address].Trusted || len(sortedDevices(devices)) != 1 {
		t.Fatal("stale advertisement remained nearby or saved row disappeared", devices[address])
	}
	updateScanDevices(devices, "[NEW] Device 11:22:33:44:55:66 Nearby headphones", true)
	rows := sortedDevices(devices)
	if len(rows) != 2 || rows[0].Address != "11:22:33:44:55:66" {
		t.Fatal("saved rows displaced nearby devices", rows)
	}
}

func TestDiscoveryOwnerLivesUntilPageCloses(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
 '')
  read -r command
  echo $$ > "$DOCK_BT_TEST_PID"
  printf 'Discovery started\n[NEW] Device 11:22:33:44:55:66 Headphones\n[CHG] Device 11:22:33:44:55:66 Icon: audio-headphones\n'
  exec /bin/sleep 60;;
 show) printf 'Controller AA:BB:CC:DD:EE:FF\n\tPowered: yes\n';;
 devices) if [ -f "$DOCK_BT_TEST_PID" ]; then read -r pid < "$DOCK_BT_TEST_PID"; if kill -0 "$pid" 2>/dev/null; then echo 'Device 11:22:33:44:55:66 Headphones'; fi; fi;;
 info) printf 'Name: Headphones\nIcon: audio-headphones\nPaired: no\nConnected: no\n';;
 *) exit 1;;
esac
`
	os.WriteFile(filepath.Join(dir, "bluetoothctl"), []byte(script), 0700)
	t.Setenv("PATH", dir)
	t.Setenv("DOCK_BT_TEST_PID", filepath.Join(dir, "scan.pid"))
	service := New()
	defer service.Close()
	task, cancel := context.WithCancel(context.Background())
	if err := service.scanBluetooth(task, 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	cancel() // Ending the initial command must not end the page-owned discovery.
	for range 2 {
		var st State
		service.readBluetooth(context.Background(), &st)
		if len(st.Devices) != 1 || !st.Devices[0].Available || !st.Devices[0].Audio {
			t.Fatal(st)
		}
	}
	if err := service.stopDiscovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	var st State
	service.readBluetooth(context.Background(), &st)
	if len(st.Devices) != 1 || st.Devices[0].Available {
		t.Fatal("closed page retained a live discovery owner", st)
	}
	if err := service.scanBluetooth(context.Background(), 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if err := service.scanBluetooth(context.Background(), time.Millisecond); err == nil {
		t.Fatal("closed service scanned")
	}
}

func TestInitialDiscoveryCancellationJoinsProcess(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bluetoothctl"), []byte("#!/bin/sh\nexec /bin/sleep 60\n"), 0700)
	t.Setenv("PATH", dir)
	service := New()
	defer service.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := service.scanBluetooth(ctx, time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}
