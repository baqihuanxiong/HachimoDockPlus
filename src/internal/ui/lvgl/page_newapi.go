//go:build lvgl && cgo

package lvgl

/*
#include "page_newapi.h"
*/
import "C"
import (
	"fmt"
	"hachimodockplus/internal/apps/newapi"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/controls"
	"strings"
)

type newapiPage struct{ countdown int }

func NewNewAPIPage() Page      { return &newapiPage{} }
func (*newapiPage) ID() string { return newapi.ID }
func (p *newapiPage) Mount() error {
	p.countdown = 0
	if C.dock_newapi_mount() != 0 {
		return fmt.Errorf("NewAPI 页面初始化失败")
	}
	return nil
}
func (p *newapiPage) Unmount() { p.countdown = 0; C.dock_newapi_unmount() }
func (p *newapiPage) HardwareEvent(e controls.Event) string {
	return newapi.HardwareAction(e, p.countdown)
}
func (p *newapiPage) Render(snapshot core.AppSnapshot) error {
	s, ok := snapshot.Data.(newapi.State)
	if !ok {
		return fmt.Errorf("NewAPI 页面收到错误状态")
	}
	p.countdown = s.Countdown

	d := s.Dashboard
	reset := fmt.Sprintf("%d 秒后执行\n每个用户余额设为 $%.2f", s.Countdown, s.ResetUSD)
	status := snapshot.Status
	if s.Result.Error != "" && !s.Resetting {
		status = s.Result.Error
	}
	withStrings([]string{status, reset}, func(cs []*C.char) {
		C.dock_newapi_render(cs[0], flag(s.Loading || s.Resetting), C.int(s.Countdown), cs[1])
	})
	for i, u := range []newapi.Usage{d.Today, d.Week, d.Month} {
		usd, calls, rate := fmt.Sprintf("$%.2f", u.USD), newapi.CompactNumber(float64(u.Calls())), newapi.RateText(u.Success, u.Failed)
		if d.Updated.IsZero() {
			usd, calls, rate = "--", "--", "--"
		}
		withStrings([]string{usd, calls, rate}, func(cs []*C.char) { C.dock_newapi_metric(C.int(i), cs[0], cs[1], cs[2]) })
	}
	for i := 0; i < 2; i++ {
		name, value, note := "", "", ""
		if i < len(d.Balances) {
			b := d.Balances[i]
			name = strings.ReplaceAll(b.Name, " Token Plan", "")
			value, note = b.Value, b.Note
			if b.Error != "" {
				value = "--"
				note = b.Error
			}
		} else if i == 0 {
			name = "尚未配置上游"
			value = "--"
		}
		withStrings([]string{name, value, note}, func(cs []*C.char) { C.dock_newapi_balance(C.int(i), cs[0], cs[1], cs[2]) })
	}
	C.dock_newapi_rows(0, C.int(len(d.Models)))
	C.dock_newapi_rows(1, C.int(len(d.Users)))
	total := d.Success + d.Failed
	for i, m := range d.Models {
		percent := 0.0
		if total > 0 {
			percent = float64(m.Calls) * 100 / float64(total)
		}
		name := m.Name
		if name == "" {
			name = "未知模型"
		}
		withStrings([]string{name, fmt.Sprintf("%.1f%%", percent)}, func(cs []*C.char) { C.dock_newapi_row(0, C.int(i), cs[0], cs[1], C.int(percent*10)) })
	}
	peak := 0.0
	if len(d.Users) > 0 {
		peak = d.Users[0].USD
	}
	for i, u := range d.Users {
		amount := 0
		if peak > 0 {
			amount = int(u.USD * 1000 / peak)
		}
		name := u.Name
		if name == "" {
			name = fmt.Sprintf("用户 #%d", u.ID)
		}
		withStrings([]string{name, fmt.Sprintf("$%.2f", u.USD)}, func(cs []*C.char) { C.dock_newapi_row(1, C.int(i), cs[0], cs[1], C.int(amount)) })
	}
	return nil
}
