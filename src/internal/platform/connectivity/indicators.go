package connectivity

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

const (
	BluetoothNone = iota
	BluetoothGeneric
	BluetoothHeadphones
	BluetoothSpeaker
	BluetoothKeyboard
	BluetoothMouse
	BluetoothGamepad
	BluetoothPhone
	BluetoothComputer
	BluetoothWatch
	BluetoothCamera
)

// Indicators are read-only header status; they never scan or configure a radio.
// WiFiStrength is 0 when disconnected and 1..4 when associated.
type Indicators struct {
	WiFiReady, WiFiConnected, BluetoothReady, BluetoothOn, BluetoothConnected bool
	WiFiStrength, BluetoothKind                                               int
}

func signalStrength(rssi int) int {
	switch {
	case rssi >= -55:
		return 4
	case rssi >= -65:
		return 3
	case rssi >= -75:
		return 2
	default:
		return 1
	}
}
func bluetoothKind(info string) int {
	var icon, class string
	for _, line := range strings.Split(info, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		switch k {
		case "Icon":
			icon = strings.TrimSpace(v)
		case "Class":
			class = strings.TrimSpace(v)
		}
	}
	switch icon {
	case "audio-headset", "audio-headphones":
		return BluetoothHeadphones
	case "audio-card", "audio-speakers", "audio-speaker":
		return BluetoothSpeaker
	case "input-keyboard":
		return BluetoothKeyboard
	case "input-mouse", "input-tablet":
		return BluetoothMouse
	case "input-gaming", "input-joystick", "input-gamepad":
		return BluetoothGamepad
	case "phone", "modem":
		return BluetoothPhone
	case "computer":
		return BluetoothComputer
	case "camera-photo", "camera-video":
		return BluetoothCamera
	case "watch":
		return BluetoothWatch
	}
	// Some devices omit Icon; fall back to the Bluetooth Class of Device.
	value, err := strconv.ParseUint(class, 0, 32)
	if err != nil {
		return BluetoothGeneric
	}
	switch (value >> 8) & 31 {
	case 1:
		return BluetoothComputer
	case 2:
		return BluetoothPhone
	case 4:
		switch (value >> 2) & 63 {
		case 1, 2, 6:
			return BluetoothHeadphones
		default:
			return BluetoothSpeaker
		}
	case 5:
		if value&0x40 != 0 {
			return BluetoothKeyboard
		}
		if value&0x80 != 0 {
			return BluetoothMouse
		}
		return BluetoothGamepad
	case 7:
		return BluetoothWatch
	}
	return BluetoothGeneric
}
func (s *Service) Indicators(ctx context.Context) Indicators {
	var st Indicators
	if out, err := s.request(ctx, "STATUS"); err == nil {
		st.WiFiReady = true
		st.WiFiConnected = fields(out)["wpa_state"] == "COMPLETED"
	}
	if st.WiFiConnected {
		st.WiFiStrength = 1
		if out, err := s.request(ctx, "SIGNAL_POLL"); err == nil {
			if rssi, err := strconv.Atoi(fields(out)["RSSI"]); err == nil && rssi < 0 && rssi >= -127 {
				st.WiFiStrength = signalStrength(rssi)
			}
		}
	}
	if out, err := bluetooth(ctx, "show"); err == nil {
		st.BluetoothReady = strings.Contains(out, "Controller ")
		st.BluetoothOn = strings.Contains(out, "Powered: yes")
	}
	if !st.BluetoothOn {
		return st
	}
	out, err := bluetooth(ctx, "devices", "Connected")
	if err != nil {
		st.BluetoothReady = false
		return st
	}
	// Sort addresses so the displayed device remains stable across BlueZ enumeration.
	addresses := []string{}
	for _, line := range strings.Split(out, "\n") {
		p := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(p) >= 2 && p[0] == "Device" && mac.MatchString(p[1]) {
			addresses = append(addresses, p[1])
		}
	}
	sort.Strings(addresses)
	// For multiple connections, show one representative device; prefer audio.
	best := 0
	for _, address := range addresses {
		info, err := bluetooth(ctx, "info", address)
		if err != nil || !strings.Contains(info, "Connected: yes") {
			continue
		}
		st.BluetoothConnected = true
		kind := bluetoothKind(info)
		priority := 1
		if kind != BluetoothGeneric {
			priority = 2
		}
		if kind == BluetoothHeadphones || kind == BluetoothSpeaker {
			priority = 3
		}
		if priority > best {
			st.BluetoothKind = kind
			best = priority
		}
		if priority == 3 {
			break
		}
	}
	return st
}
