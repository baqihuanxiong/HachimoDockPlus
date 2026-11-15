// Package bluez handles device connections across upstream and Rockchip BlueZ.
package bluez

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const audioSink = "0000110b-0000-1000-8000-00805f9b34fb"

var addressPattern = regexp.MustCompile(`(?i)^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

type command func(context.Context, ...string) ([]byte, error)

func run(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "dbus-send", args...).CombinedOutput()
}

// ConnectAudio selects the remote audio sink rather than all advertised profiles.
func ConnectAudio(ctx context.Context, address string) error {
	return deviceCall(ctx, address, "ConnectProfile", run, "string:"+audioSink)
}

func Connect(ctx context.Context, address string) error {
	return deviceCall(ctx, address, "Connect", run)
}

func Disconnect(ctx context.Context, address string) error {
	return deviceCall(ctx, address, "Disconnect", run)
}

func deviceCall(ctx context.Context, address, method string, execute command, params ...string) error {
	if !addressPattern.MatchString(address) {
		return fmt.Errorf("蓝牙设备地址无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	path := "/org/bluez/hci0/dev_" + strings.ReplaceAll(strings.ToUpper(address), ":", "_")
	args := []string{"--system", "--dest=org.bluez", "--type=method_call", "--print-reply", "--reply-timeout=24000", path, "org.bluez.Device1." + method}
	args = append(args, params...)
	out, err := execute(ctx, args...)
	// Rockchip's BlueZ patch adds ADDR_TYPE to Connect and Disconnect, while
	// the supplied bluetoothctl still calls the upstream argument-free API.
	if err != nil && len(params) == 0 && (method == "Connect" || method == "Disconnect") && strings.Contains(string(out), "org.freedesktop.DBus.Error.UnknownMethod") {
		out, err = execute(ctx, append(args, "string:auto")...)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil && method == "ConnectProfile" && strings.Contains(string(out), "org.bluez.Error.AlreadyConnected") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("蓝牙 %s 失败: %s: %w", method, strings.TrimSpace(string(out)), err)
	}
	return nil
}
