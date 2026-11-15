package connectivity

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"hachimodockplus/internal/platform/audio"
	"hachimodockplus/internal/platform/bluez"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
var mac = regexp.MustCompile(`(?i)^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

func bluetooth(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bluetoothctl", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TERM=dumb")
	out, err := cmd.CombinedOutput()
	text := ansi.ReplaceAllString(string(out), "")
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil || strings.Contains(text, "Failed") || strings.Contains(text, "failed") || strings.Contains(text, "not available") || strings.Contains(text, "No default controller") {
		return "", errors.New("蓝牙操作失败，请检查设备电源、配对模式和系统 BlueZ 服务")
	}
	return text, nil
}

// BlueZ may expire unpaired objects after discovery stops. Keep the last scan
// until an explicit new scan or power-off, and refresh connection flags separately.
func (s *Service) readBluetooth(ctx context.Context, st *State) {
	out, err := bluetooth(ctx, "show")
	if err != nil {
		st.BluetoothError = err.Error()
		s.cachedBluetooth(st)
		return
	}
	st.BluetoothReady = strings.Contains(out, "Controller ")
	st.BluetoothOn = strings.Contains(out, "Powered: yes")
	if !st.BluetoothOn {
		s.btMu.Lock()
		s.btDevices = nil
		s.btMu.Unlock()
		return
	}
	out, err = bluetooth(ctx, "devices")
	if err != nil {
		st.BluetoothError = err.Error()
		s.cachedBluetooth(st)
		return
	}
	s.btMu.Lock()
	defer s.btMu.Unlock()
	if s.btDevices == nil {
		s.btDevices = map[string]Device{}
	}
	for address, d := range s.btDevices {
		d.Available = false
		d.Connected = false
		s.btDevices[address] = d
	}
	for _, line := range strings.Split(out, "\n") {
		p := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(p) < 3 || p[0] != "Device" || !mac.MatchString(p[1]) {
			continue
		}
		address := strings.ToUpper(p[1])
		if _, exists := s.btDevices[address]; !exists && len(s.btDevices) >= 128 {
			continue
		}
		d := s.btDevices[address]
		d.Address = address
		d.Available = true
		if friendlyName(p[2]) != "" {
			d.Name = friendlyName(p[2])
		}
		task, cancel := context.WithTimeout(ctx, time.Second)
		info, e := bluetooth(task, "info", address)
		cancel()
		if e == nil {
			d = deviceInfo(d, info)
		}
		s.btDevices[address] = d
	}
	st.Devices = sortedDevices(s.btDevices)
}
func (s *Service) cachedBluetooth(st *State) {
	s.btMu.Lock()
	defer s.btMu.Unlock()
	st.Devices = sortedDevices(s.btDevices)
	for i := range st.Devices {
		st.Devices[i].Connected = false
		st.Devices[i].Available = false
	}
}
func friendlyName(name string) string {
	name = strings.TrimSpace(decodeSSID(name))
	if !utf8.ValidString(name) {
		return ""
	}
	compact := strings.NewReplacer(":", "", "-", "", " ", "").Replace(name)
	if compact == "" {
		return ""
	}
	if len(compact) >= 12 {
		hex := true
		for _, c := range compact {
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				hex = false
				break
			}
		}
		if hex {
			return ""
		}
	}
	return name
}
func deviceInfo(d Device, info string) Device {
	var alias, name string
	for _, line := range strings.Split(info, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "Alias":
			alias = friendlyName(value)
		case "Name":
			name = friendlyName(value)
		case "Paired":
			d.Paired = value == "yes"
		case "Connected":
			d.Connected = value == "yes"
		case "Trusted":
			d.Trusted = value == "yes"
		}
	}
	if alias != "" {
		d.Name = alias
	} else if name != "" {
		d.Name = name
	}
	kind := bluetoothKind(info)
	if kind != BluetoothGeneric {
		d.Kind = kind
	}
	d.Audio = d.Audio || d.Kind == BluetoothHeadphones || d.Kind == BluetoothSpeaker || strings.Contains(strings.ToLower(info), "0000110b-")
	return d
}
func sortedDevices(devices map[string]Device) []Device {
	out := make([]Device, 0, len(devices))
	for _, d := range devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Connected != b.Connected {
			return a.Connected
		}
		if a.Audio != b.Audio {
			return a.Audio
		}
		if a.Paired != b.Paired {
			return a.Paired
		}
		if (a.Name != "") != (b.Name != "") {
			return a.Name != ""
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Address < b.Address
	})
	if len(out) > 24 {
		out = out[:24]
	}
	for i := range out {
		if out[i].Name == "" {
			out[i].Name = "未命名设备"
			if out[i].Audio {
				out[i].Name = "未命名音频设备"
			}
		}
	}
	return out
}

// scanDevices retains names and properties emitted while discovery is alive,
// even if BlueZ removes those temporary objects as bluetoothctl disconnects.
func updateScanDevices(devices map[string]Device, out string) {
	for _, line := range strings.Split(out, "\n") {
		at := strings.Index(line, "Device ")
		if at < 0 {
			continue
		}
		p := strings.SplitN(strings.TrimSpace(line[at+7:]), " ", 2)
		if len(p) != 2 || !mac.MatchString(p[0]) {
			continue
		}
		address := strings.ToUpper(p[0])
		d := devices[address]
		d.Address = address
		d.Available = true
		if strings.Contains(line[:at], "[DEL]") {
			d.Available = false
			devices[address] = d
			continue
		}
		if strings.Contains(line[:at], "[CHG]") {
			d = deviceInfo(d, p[1])
		} else if name := friendlyName(p[1]); name != "" {
			d.Name = name
		}
		if len(devices) < 128 || devices[address].Address != "" {
			devices[address] = d
		}
	}
}
func (s *Service) Bluetooth(ctx context.Context, action, address string) error {
	var err error
	switch action {
	case "on", "off":
		if action == "off" {
			if err := s.stopDiscovery(ctx); err != nil {
				return err
			}
		}
		_, err = bluetooth(ctx, "power", action)
	case "scan":
		return s.scanBluetooth(ctx, 15*time.Second)
	case "pair", "connect", "disconnect", "remove":
		if !mac.MatchString(address) {
			return errors.New("蓝牙设备地址无效")
		}
		if action == "pair" || action == "connect" {
			if _, e := os.Stat("/etc/init.d/S45hachimodock-audio"); e == nil {
				if e = audio.EnsureBluetooth(ctx); e != nil {
					return e
				}
			}
		}
		if action == "pair" {
			info, e := bluetooth(ctx, "info", address)
			if e != nil {
				return e
			}
			if strings.Contains(info, "Paired: yes") {
				return errors.New("设备已配对，请直接连接")
			}
			_, err = bluetooth(ctx, "--agent", "NoInputNoOutput", "pair", address)
		} else if action == "connect" {
			info, e := bluetooth(ctx, "info", address)
			if e != nil {
				return e
			}
			if deviceInfo(Device{}, info).Audio {
				err = bluez.ConnectAudio(ctx, address)
			} else {
				err = bluez.Connect(ctx, address)
			}
		} else if action == "disconnect" {
			err = bluez.Disconnect(ctx, address)
		} else {
			_, err = bluetooth(ctx, action, address)
		}
	default:
		return errors.New("未知蓝牙操作")
	}
	if err == nil && (action == "pair" || action == "connect") {
		_, err = bluetooth(ctx, "trust", address)
	}
	if err == nil {
		s.btMu.Lock()
		switch action {
		case "off":
			s.btDevices = nil
		case "remove":
			delete(s.btDevices, strings.ToUpper(address))
		case "disconnect":
			d := s.btDevices[strings.ToUpper(address)]
			if d.Address != "" {
				d.Connected = false
				s.btDevices[d.Address] = d
			}
		}
		s.btMu.Unlock()
	}
	return err
}
