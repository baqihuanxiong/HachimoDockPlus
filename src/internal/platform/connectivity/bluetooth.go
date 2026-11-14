package connectivity

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
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
func (s *Service) readBluetooth(ctx context.Context, st *State) {
	out, err := bluetooth(ctx, "show")
	if err != nil {
		st.BluetoothError = err.Error()
		return
	}
	st.BluetoothReady = strings.Contains(out, "Controller ")
	st.BluetoothOn = strings.Contains(out, "Powered: yes")
	out, err = bluetooth(ctx, "devices")
	if err != nil {
		st.BluetoothError = err.Error()
		return
	}
	for _, line := range strings.Split(out, "\n") {
		p := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(p) < 3 || p[0] != "Device" || !mac.MatchString(p[1]) {
			continue
		}
		if len(st.Devices) >= 24 {
			break
		}
		d := Device{Address: p[1], Name: p[2]}
		info, e := bluetooth(ctx, "info", d.Address)
		if e != nil {
			continue
		}
		d.Paired = strings.Contains(info, "Paired: yes")
		d.Connected = strings.Contains(info, "Connected: yes")
		d.Trusted = strings.Contains(info, "Trusted: yes")
		st.Devices = append(st.Devices, d)
	}
}
func (s *Service) Bluetooth(ctx context.Context, action, address string) error {
	var err error
	switch action {
	case "on", "off":
		_, err = bluetooth(ctx, "power", action)
	case "scan":
		_, err = bluetooth(ctx, "--timeout", "8", "scan", "on")
	case "pair", "connect", "disconnect", "remove":
		if !mac.MatchString(address) {
			return errors.New("蓝牙设备地址无效")
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
		} else {
			_, err = bluetooth(ctx, action, address)
		}
	default:
		return errors.New("未知蓝牙操作")
	}
	return err
}
