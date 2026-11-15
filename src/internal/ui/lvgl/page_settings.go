//go:build lvgl && cgo

package lvgl

/*
#include "page_settings.h"
*/
import "C"
import (
	"fmt"
	"reflect"
	"strings"

	"hachimodockplus/internal/apps/settings"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/connectivity"
	"hachimodockplus/internal/preferences"
)

type settingsPage struct {
	previous connectivity.State
	rendered bool
}

func NewSettingsPage() Page      { return &settingsPage{} }
func (*settingsPage) ID() string { return settings.ID }
func (p *settingsPage) Mount() error {
	p.rendered = false
	if C.dock_settings_mount() != 0 {
		return fmt.Errorf("设置页面初始化失败")
	}
	for _, theme := range preferences.Themes() {
		withStrings([]string{theme.ID, theme.Title}, func(cs []*C.char) { C.dock_settings_theme_add(cs[0], cs[1], C.uint(theme.Color)) })
	}
	return nil
}
func (*settingsPage) Back() bool { return C.dock_settings_back() != 0 }
func (*settingsPage) Unmount()   { C.dock_settings_unmount() }
func flag(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
func (p *settingsPage) Render(snapshot core.AppSnapshot) error {
	s, ok := snapshot.Data.(settings.State)
	if !ok {
		return fmt.Errorf("设置页面收到错误状态")
	}
	withStrings([]string{s.Theme, s.Mode}, func(cs []*C.char) { C.dock_settings_theme(cs[0]); C.dock_settings_mode(cs[1]) })
	withStrings([]string{s.Screensaver}, func(cs []*C.char) { C.dock_settings_screensaver(cs[0]) })
	withStrings([]string{s.Volume.Error}, func(cs []*C.char) { C.dock_settings_volume(C.int(s.Volume.Volume), cs[0]) })
	st := s.Connectivity
	labels, addresses := []string{}, []string{}
	for _, d := range st.Devices {
		if d.Paired && d.Audio {
			labels = append(labels, strings.NewReplacer("\n", " ", "\r", " ").Replace(d.Name)+" ("+d.Address+")")
			addresses = append(addresses, d.Address)
		}
	}
	if s.Volume.BluetoothDevice != "" {
		found := false
		for _, address := range addresses {
			if strings.EqualFold(address, s.Volume.BluetoothDevice) {
				found = true
			}
		}
		if !found {
			labels = append(labels, "已保存设备 ("+s.Volume.BluetoothDevice+")")
			addresses = append(addresses, s.Volume.BluetoothDevice)
		}
	}
	withStrings([]string{s.Volume.Output, s.Volume.BluetoothDevice, strings.Join(labels, "\n"), strings.Join(addresses, "\n")}, func(cs []*C.char) { C.dock_settings_audio(cs[0], cs[1], cs[2], cs[3]) })
	wifi := "未连接"
	if st.SSID != "" {
		wifi = st.SSID + "  " + st.IP
	}
	if st.WiFiError != "" {
		wifi = st.WiFiError
	}
	bt := "蓝牙已关闭"
	if st.BluetoothOn {
		bt = "蓝牙已开启"
	}
	if st.BluetoothError != "" {
		bt = st.BluetoothError
	}
	withStrings([]string{snapshot.Status, wifi, bt}, func(cs []*C.char) {
		C.dock_settings_render(cs[0], cs[1], cs[2], flag(st.BluetoothOn), flag(st.BluetoothReady), flag(s.Busy))
	})
	summary := "共享已关闭"
	if !s.System.PasswordSet {
		summary = "请先设置共享账号"
	} else if s.System.Running {
		summary = "已开启 · 用户名：" + s.System.Username
	}
	if s.System.Error != "" {
		summary = s.System.Error
	}
	address := "smb://开发板IP/" + s.System.ShareName
	if st.IP != "" {
		address = fmt.Sprintf("\\\\%s\\%s", st.IP, s.System.ShareName)
	}
	withStrings([]string{s.System.Username, summary, address}, func(cs []*C.char) {
		C.dock_settings_system(flag(s.System.Available), flag(s.System.Enabled), flag(s.System.CanRestart), cs[0], cs[1], cs[2])
	})
	// Keep list objects stable while a command runs; only rebuild on fresh results.
	if !s.Busy && (!p.rendered || !reflect.DeepEqual(p.previous.Networks, st.Networks) || !reflect.DeepEqual(p.previous.Devices, st.Devices)) {
		p.previous = st
		p.rendered = true
		C.dock_settings_clear()
		for _, n := range st.Networks {
			withStrings([]string{n.ID, n.SSID, n.Security}, func(cs []*C.char) { C.dock_settings_network(cs[0], cs[1], cs[2], C.int(n.Signal), flag(n.Current)) })
		}
		for _, d := range st.Devices {
			withStrings([]string{d.Address, d.Name, bluetoothType(d)}, func(cs []*C.char) {
				C.dock_settings_device(cs[0], cs[1], cs[2], flag(d.Paired), flag(d.Connected), flag(d.Trusted), flag(d.Available))
			})
		}
	}
	return nil
}

func bluetoothType(d connectivity.Device) string {
	switch d.Kind {
	case connectivity.BluetoothHeadphones:
		return "耳机"
	case connectivity.BluetoothSpeaker:
		return "音箱"
	case connectivity.BluetoothKeyboard:
		return "键盘"
	case connectivity.BluetoothMouse:
		return "鼠标"
	case connectivity.BluetoothPhone:
		return "手机"
	}
	if d.Audio {
		return "音频设备"
	}
	return "蓝牙设备"
}
