// Package newapi owns the foreground gateway dashboard and reset state machine.
package newapi

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/controls"
)

const ID = "newapi-monitor"
const (
	ActionRefresh = "refresh"
	ActionReset   = "reset.begin"
	ActionConfirm = "reset.confirm"
	ActionCancel  = "reset.cancel"
)

var Manifest = core.Manifest{ID: ID, Title: "NewAPI 监控", Description: "用量概览 · 上游余额 · 排行", Icon: "N", IconImage: "assets/app-icons/newapi.png", Capabilities: []core.Capability{core.Network}}

type State struct {
	Dashboard          Dashboard
	Loading, Resetting bool
	Countdown          int
	ResetUSD           float64
	Result             ResetResult
}
type App struct {
	mu                sync.Mutex
	wg                sync.WaitGroup
	closeOnce         sync.Once
	closeErr          error
	service           Service
	cfg               Config
	ctx               context.Context
	cancel            context.CancelFunc
	countCancel       context.CancelFunc
	epoch, countEpoch uint64
	revision          uint64
	closed            bool
	state             State
	status            string
}

func New(c Config, s Service) *App {
	return &App{cfg: c, service: s, state: State{ResetUSD: c.ResetUSD}, status: "等待刷新"}
}

func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return core.ErrClosed
	}
	a.stopLocked()
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.epoch++
	a.state.Countdown = 0
	a.state.Resetting = false
	a.state.Loading = false
	a.refreshLocked()
	ctx = a.ctx
	epoch := a.epoch
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(time.Duration(a.cfg.RefreshSeconds) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.mu.Lock()
				if a.valid(epoch) && !a.state.Loading && !a.state.Resetting && a.state.Countdown == 0 {
					a.refreshLocked()
				}
				a.mu.Unlock()
			}
		}
	}()
	return nil
}
func (a *App) valid(epoch uint64) bool {
	return a.ctx != nil && a.ctx.Err() == nil && epoch == a.epoch && !a.closed
}
func (a *App) stopLocked() {
	if a.state.Resetting {
		a.state.Result.Error = "离开页面已停止后续重置；已发送请求可能已生效，请核对余额"
	}
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	if a.countCancel != nil {
		a.countCancel()
		a.countCancel = nil
	}
	a.ctx = nil
	a.epoch++
	a.countEpoch++
	a.state.Countdown = 0
	a.state.Loading = false
	a.state.Resetting = false
	a.revision++
}
func (a *App) Stop() { a.mu.Lock(); a.stopLocked(); a.mu.Unlock() }
func (a *App) Close() error {
	a.mu.Lock()
	a.closed = true
	a.stopLocked()
	a.mu.Unlock()
	a.wg.Wait()
	a.closeOnce.Do(func() {
		if closer, ok := a.service.(interface{ Close() error }); ok {
			a.closeErr = closer.Close()
		}
	})
	return a.closeErr
}

func (a *App) refreshLocked() {
	if a.state.Loading || a.state.Resetting || a.state.Countdown > 0 {
		return
	}
	a.state.Loading = true
	a.status = "正在读取用量数据…"
	a.revision++
	ctx := a.ctx
	epoch := a.epoch
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		d, err := a.service.Fetch(ctx)
		a.mu.Lock()
		defer a.mu.Unlock()
		if !a.valid(epoch) {
			return
		}
		a.state.Loading = false
		if err != nil {
			a.status = "刷新失败：" + err.Error()
		} else {
			a.state.Dashboard = d
			a.status = "最近更新 " + d.Updated.In(mustLocation(a.cfg.Timezone)).Format("01-02 15:04:05")
			if d.Partial {
				a.status += " · 日志达到上限，统计不完整"
			}
		}
		a.revision++
	}()
}
func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

func (a *App) Handle(e core.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return core.ErrClosed
	}
	if a.ctx == nil || a.ctx.Err() != nil {
		return core.ErrInactive
	}
	switch e.Action {
	case ActionRefresh:
		a.refreshLocked()
	case ActionReset:
		if a.state.Loading || a.state.Resetting || a.state.Countdown > 0 {
			return nil
		}
		a.state.Result = ResetResult{}
		a.state.Countdown = 10
		a.status = "10 秒后重置所有用户余额；SW2 立即执行，SW3 取消"
		a.revision++
		a.countEpoch++
		generation := a.countEpoch
		epoch := a.epoch
		ctx, cancel := context.WithCancel(a.ctx)
		a.countCancel = cancel
		deadline := time.Now().Add(10 * time.Second)
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					a.mu.Lock()
					if !a.valid(epoch) || generation != a.countEpoch || a.state.Countdown == 0 {
						a.mu.Unlock()
						return
					}
					remaining := max(0, int(math.Ceil(deadline.Sub(now).Seconds())))
					if remaining == 0 {
						a.executeLocked()
						a.mu.Unlock()
						return
					}
					if remaining != a.state.Countdown {
						a.state.Countdown = remaining
						a.revision++
					}
					a.mu.Unlock()
				}
			}
		}()
	case ActionCancel:
		if a.state.Countdown > 0 {
			a.countEpoch++
			if a.countCancel != nil {
				a.countCancel()
				a.countCancel = nil
			}
			a.state.Countdown = 0
			a.status = "已取消重置，未修改余额"
			a.revision++
		}
	case ActionConfirm:
		if a.state.Countdown > 0 {
			a.executeLocked()
		}
	default:
		return core.ErrUnknownAction
	}
	return nil
}

func shortName(s string) string {
	r := []rune(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " "))
	if len(r) > 50 {
		return string(r[:50]) + "…"
	}
	return string(r)
}
func RateText(success, failed int) string {
	if success+failed == 0 {
		return "--"
	}
	return fmt.Sprintf("%.1f%%", rate(success, failed))
}

func (a *App) executeLocked() {
	if a.state.Countdown == 0 || a.state.Resetting {
		return
	}
	if a.countCancel != nil {
		a.countCancel()
		a.countCancel = nil
	}
	a.countEpoch++
	a.state.Countdown = 0
	a.state.Resetting = true
	a.status = "正在重置所有用户余额…"
	a.revision++
	epoch := a.epoch
	ctx := a.ctx
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		r := a.service.Reset(ctx, a.cfg.ResetUSD, func(r ResetResult) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.valid(epoch) {
				a.state.Result = r
				a.status = fmt.Sprintf("已核对 %d / %d 个用户", r.Done, r.Total)
				a.revision++
			}
		})
		a.mu.Lock()
		defer a.mu.Unlock()
		if !a.valid(epoch) {
			return
		}
		a.state.Resetting = false
		a.state.Result = r
		if r.Error != "" {
			a.status = fmt.Sprintf("重置停止：已核对 %d/%d；%s", r.Done, r.Total, r.Error)
		} else {
			a.status = fmt.Sprintf("重置完成：%d 个用户余额设为 $%.2f", r.Done, a.cfg.ResetUSD)
		}
		a.revision++
	}()
}

func HardwareAction(e controls.Event, countdown int) string {
	if !e.Pressed || countdown == 0 {
		return ""
	}
	switch e.Key {
	case controls.SW2:
		return ActionConfirm
	case controls.SW3:
		return ActionCancel
	}
	return ""
}

func (a *App) Snapshot() core.AppSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.state
	d := &s.Dashboard
	d.Models = append([]Model(nil), d.Models...)
	d.Users = append([]UserUsage(nil), d.Users...)
	d.Balances = append([]Balance(nil), d.Balances...)
	return core.AppSnapshot{Revision: a.revision, Status: a.status, Data: s, Text: DashboardText(s)}
}

func DashboardText(s State) string {
	d := s.Dashboard
	var b strings.Builder
	for _, period := range []struct {
		name  string
		usage Usage
	}{{"今日", d.Today}, {"本周", d.Week}, {"本月", d.Month}} {
		fmt.Fprintf(&b, "%s $%.2f · %d 次 · 成功 %s\n", period.name, period.usage.USD, period.usage.Calls(), RateText(period.usage.Success, period.usage.Failed))
	}
	for _, p := range d.Balances {
		fmt.Fprintf(&b, "%s %s %s%s\n", shortName(p.Name), p.Value, p.Note, p.Error)
	}
	for _, m := range d.Models {
		fmt.Fprintf(&b, "%s %d 次\n", shortName(m.Name), m.Calls)
	}
	for _, u := range d.Users {
		fmt.Fprintf(&b, "%s $%.2f\n", shortName(u.Name), u.USD)
	}
	if s.Result.Error != "" {
		fmt.Fprintln(&b, s.Result.Error)
	}
	return b.String()
}

func CompactNumber(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.2fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1fk", v/1e3)
	}
	return fmt.Sprintf("%.0f", v)
}
