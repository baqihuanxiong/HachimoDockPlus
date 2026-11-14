//go:build lvgl && cgo

package lvgl

/*
#include "page_settings.h"
*/
import "C"
import (
	"fmt"
	"reflect"

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
func (*settingsPage) Unmount() { C.dock_settings_unmount() }
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
	st := s.Connectivity
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
	// Keep list objects stable while a command runs; only rebuild on fresh results.
	if !s.Busy && (!p.rendered || !reflect.DeepEqual(p.previous.Networks, st.Networks) || !reflect.DeepEqual(p.previous.Devices, st.Devices)) {
		p.previous = st
		p.rendered = true
		C.dock_settings_clear()
		for _, n := range st.Networks {
			withStrings([]string{n.ID, n.SSID, n.Security}, func(cs []*C.char) { C.dock_settings_network(cs[0], cs[1], cs[2], C.int(n.Signal), flag(n.Current)) })
		}
		for _, d := range st.Devices {
			withStrings([]string{d.Address, d.Name}, func(cs []*C.char) { C.dock_settings_device(cs[0], cs[1], flag(d.Paired), flag(d.Connected)) })
		}
	}
	return nil
}
