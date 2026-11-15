package audio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"hachimodockplus/internal/platform/bluez"
)

type OutputDevices struct{ Speaker, SPDIF string }

var addressPattern = regexp.MustCompile(`(?i)^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

// NewConfiguredVolume shares one preference and output route across all players.
func NewConfiguredVolume(initial VolumeState, devices OutputDevices, mixer Mixer, persist func(VolumeState) error) (*Volume, error) {
	if initial.Output == "" {
		initial.Output = "speaker"
	}
	if initial.Volume < 0 || initial.Volume > 100 {
		return nil, errors.New("音量必须为 0～100")
	}
	if err := validateOutput(initial.Output, initial.BluetoothDevice); err != nil {
		return nil, err
	}
	v := &Volume{mixer: mixer, devices: devices, persist: persist, state: initial, wake: make(chan struct{}, 1), done: make(chan struct{})}
	v.state.Revision = 1
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	go v.run(ctx)
	return v, nil
}
func validateOutput(output, address string) error {
	switch output {
	case "speaker", "spdif":
	case "bluetooth":
		if !addressPattern.MatchString(address) {
			return errors.New("请先选择已配对的蓝牙耳机或音箱")
		}
	default:
		return errors.New("无效的音频输出")
	}
	return nil
}
func (v *Volume) SetOutput(output, address string) error {
	if err := validateOutput(output, address); err != nil {
		return err
	}
	v.mu.Lock()
	v.state.Output = output
	if address != "" {
		v.state.BluetoothDevice = strings.ToUpper(address)
	}
	v.state.Error = ""
	v.state.Revision++
	v.dirty = true
	v.mu.Unlock()
	v.Initialize()
	return nil
}
func (v *Volume) configured() bool { return v.Snapshot().Output != "" }
func (v *Volume) PlaybackDevice(fallback string) string {
	s := v.Snapshot()
	switch s.Output {
	case "speaker":
		if v.devices.Speaker != "" {
			return v.devices.Speaker
		}
	case "spdif":
		return "plug:SLAVE=" + strconv.Quote(v.devices.SPDIF)
	case "bluetooth":
		return "plug:SLAVE=" + strconv.Quote("bluealsa:DEV="+s.BluetoothDevice+",PROFILE=a2dp")
	}
	if fallback == "" {
		return "default"
	}
	return fallback
}

// The board ships BlueALSA and its ALSA plugin, but no audio source service.
func EnsureBluetooth(ctx context.Context) error {
	if err := exec.CommandContext(ctx, "/etc/init.d/S45hachimodock-audio", "start").Run(); err != nil {
		return errors.New("蓝牙音频服务不可用，请安装 S45hachimodock-audio 并检查 BlueALSA")
	}
	return nil
}
func gpio(number int, enabled bool) error {
	path := fmt.Sprintf("/sys/class/gpio/gpio%d", number)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err = os.WriteFile("/sys/class/gpio/export", []byte(fmt.Sprint(number)), 0200); err != nil {
			if _, statErr := os.Stat(path); statErr != nil {
				return err
			}
		}
	}
	level := "low"
	if enabled {
		level = "high"
	}
	return os.WriteFile(path+"/direction", []byte(level), 0200)
}
func (v *Volume) route(ctx context.Context) error {
	s := v.Snapshot()
	// Disable unused physical outputs before enabling the selected one.
	if err := gpio(12, false); err != nil {
		return errors.New("无法关闭扬声器功放")
	}
	if err := gpio(13, s.Output == "spdif"); err != nil {
		return errors.New("无法设置 S/PDIF 输出使能")
	}
	switch s.Output {
	case "speaker":
		if err := gpio(12, true); err != nil {
			return errors.New("无法开启扬声器功放")
		}
	case "bluetooth":
		if err := EnsureBluetooth(ctx); err != nil {
			return err
		}
		if err := bluez.ConnectAudio(ctx, s.BluetoothDevice); err != nil {
			return errors.New("无法连接蓝牙音频设备，请在蓝牙页配对并连接")
		}
	}
	return nil
}
func (v *Volume) releaseRoute() {
	_ = gpio(12, false)
	_ = gpio(13, false)
}
