// Package settings owns foreground connectivity tasks and their immutable UI state.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/audio"
	"hachimodockplus/internal/platform/connectivity"
	"hachimodockplus/internal/platform/system"
	"hachimodockplus/internal/preferences"
)

const ID = "settings"

var Manifest = core.Manifest{ID: ID, Title: "设置", Description: "网络、蓝牙、系统与外观", Icon: "\uf013", Capabilities: []core.Capability{core.Network}}

type SystemBackend interface {
	Read(context.Context) system.State
	ConfigureSMB(context.Context, system.Credentials) error
	SetSMB(context.Context, bool) error
	Restart(context.Context) error
}

type Backend interface {
	Read(context.Context) connectivity.State
	ScanWiFi(context.Context) error
	JoinWiFi(context.Context, connectivity.Join) error
	ConnectSaved(context.Context, string) error
	DisconnectWiFi(context.Context) error
	ForgetWiFi(context.Context, string) error
	Bluetooth(context.Context, string, string) error
}
type State struct {
	Volume       audio.VolumeState
	Connectivity connectivity.State
	Theme        string
	Screensaver  string
	Mode         string
	Busy         bool
	System       system.State
}
type App struct {
	mu              sync.Mutex
	worker          sync.Mutex
	volume          *audio.Volume
	backend         Backend
	system          SystemBackend
	preferences     *preferences.Store
	state           State
	status          string
	revision, epoch uint64
	cancel          context.CancelFunc
	events          chan core.Event
	wg              sync.WaitGroup
	closed          bool
}

func New(b Backend, prefs *preferences.Store, volumes ...*audio.Volume) *App {
	a := &App{backend: b, preferences: prefs, status: "读取连接状态…"}
	if len(volumes) > 0 {
		a.volume = volumes[0]
	}
	return a
}
func NewWithSystem(b Backend, prefs *preferences.Store, volume *audio.Volume, backend SystemBackend) *App {
	a := New(b, prefs, volume)
	a.system = backend
	return a
}
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return core.ErrClosed
	}
	if a.cancel != nil {
		return nil
	}
	ctx, a.cancel = context.WithCancel(ctx)
	a.epoch++
	epoch := a.epoch
	a.events = make(chan core.Event, 1)
	events := a.events
	a.state.Busy = true
	a.revision++
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.worker.Lock()
		defer a.worker.Unlock()
		if ctx.Err() != nil {
			return
		}
		a.refresh(ctx, epoch, "准备就绪")
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-events:
				task, cancel := context.WithTimeout(ctx, 40*time.Second)
				err := a.perform(task, e)
				cancel()
				if ctx.Err() != nil {
					return
				}
				status := "操作完成"
				if err != nil {
					status = err.Error()
					if errors.Is(err, context.DeadlineExceeded) {
						status = "操作超时，请检查密码或设备配对模式"
					}
				}
				if e.Action == "theme.set" || e.Action == "mode.set" || e.Action == "screensaver.set" {
					a.mu.Lock()
					if epoch == a.epoch {
						a.state.Busy = false
						a.status = status
						a.revision++
					}
					a.mu.Unlock()
				} else {
					a.refresh(ctx, epoch, status)
				}
			case <-ticker.C:
				a.refresh(ctx, epoch, "")
			}
		}
	}()
	return nil
}
func (a *App) refresh(ctx context.Context, epoch uint64, status string) {
	read, cancel := context.WithTimeout(ctx, 12*time.Second)
	st := a.backend.Read(read)
	cancel()
	sys := system.State{Username: "dockconfig", ShareName: system.ShareName, Error: "系统服务不可用"}
	if a.system != nil {
		read, cancel := context.WithTimeout(ctx, 5*time.Second)
		sys = a.system.Read(read)
		cancel()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if ctx.Err() != nil || a.closed || epoch != a.epoch {
		return
	}
	a.state.Connectivity = st
	a.state.System = sys
	// A periodic read must not clear a command queued while it was in progress.
	if len(a.events) == 0 {
		a.state.Busy = false
	}
	if status != "" {
		a.status = status
	}
	a.revision++
}
func (a *App) perform(ctx context.Context, e core.Event) error {
	switch e.Action {
	case "audio.output", "audio.bluetooth":
		if a.volume == nil {
			return errors.New("音频服务不可用")
		}
		if e.Action == "audio.output" {
			if e.Text != "speaker" && e.Text != "spdif" {
				return errors.New("无效的音频输出")
			}
			return a.volume.SetOutput(e.Text, "")
		}
		a.mu.Lock()
		devices := append([]connectivity.Device(nil), a.state.Connectivity.Devices...)
		a.mu.Unlock()
		for _, d := range devices {
			if strings.EqualFold(d.Address, e.Text) && d.Paired && d.Audio {
				return a.volume.SetOutput("bluetooth", d.Address)
			}
		}
		return errors.New("请先在蓝牙页配对耳机或音箱，再选择输出")
	case "smb.account":
		if a.system == nil {
			return errors.New("系统服务不可用")
		}
		var c system.Credentials
		if json.Unmarshal([]byte(e.Text), &c) != nil {
			return errors.New("共享账号输入无效")
		}
		return a.system.ConfigureSMB(ctx, c)
	case "smb.set":
		if a.system == nil {
			return errors.New("系统服务不可用")
		}
		if e.Text != "true" && e.Text != "false" {
			return errors.New("共享开关值无效")
		}
		return a.system.SetSMB(ctx, e.Text == "true")
	case "system.restart":
		if a.system == nil {
			return errors.New("系统服务不可用")
		}
		if a.volume != nil {
			if err := a.volume.Flush(); err != nil {
				return errors.New("音频设置尚未保存，请稍后重试")
			}
		}
		return a.system.Restart(ctx)
	case "refresh":
		return nil
	case "theme.set":
		return a.preferences.SetTheme(e.Text)
	case "screensaver.set":
		return a.preferences.SetScreensaver(e.Text)
	case "mode.set":
		return a.preferences.SetMode(e.Text)
	case "wifi.scan":
		return a.backend.ScanWiFi(ctx)
	case "wifi.join":
		var j connectivity.Join
		if json.Unmarshal([]byte(e.Text), &j) != nil {
			return errors.New("Wi-Fi 输入无效")
		}
		return a.backend.JoinWiFi(ctx, j)
	case "wifi.connect":
		return a.backend.ConnectSaved(ctx, e.Text)
	case "wifi.disconnect":
		return a.backend.DisconnectWiFi(ctx)
	case "wifi.forget":
		return a.backend.ForgetWiFi(ctx, e.Text)
	}
	return a.backend.Bluetooth(ctx, strings.TrimPrefix(e.Action, "bt."), e.Text)
}
func (a *App) Handle(e core.Event) error {
	switch e.Action {
	case "volume.set", "volume.up", "volume.down":
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.closed || a.cancel == nil {
			return core.ErrInactive
		}
		if a.volume == nil {
			return errors.New("音频服务不可用")
		}
		switch e.Action {
		case "volume.set":
			n, err := strconv.Atoi(e.Text)
			if err != nil {
				return errors.New("音量无效")
			}
			a.volume.Set(n)
		case "volume.up":
			a.volume.Adjust(5)
		case "volume.down":
			a.volume.Adjust(-5)
		}
		return nil
	case "audio.output", "audio.bluetooth", "smb.account", "smb.set", "system.restart", "screensaver.set", "mode.set", "theme.set", "refresh", "wifi.scan", "wifi.join", "wifi.connect", "wifi.disconnect", "wifi.forget", "bt.on", "bt.off", "bt.scan", "bt.pair", "bt.connect", "bt.disconnect", "bt.remove":
	default:
		return core.ErrUnknownAction
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.cancel == nil {
		return core.ErrInactive
	}
	if a.state.Busy {
		return errors.New("正在处理，请稍候")
	}
	a.state.Busy = true
	a.status = "正在处理…"
	a.revision++
	a.events <- e
	return nil
}
func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	if scanner, ok := a.backend.(interface{ StopDiscovery() }); ok {
		scanner.StopDiscovery()
	}
	a.epoch++
	a.state.Busy = false
	a.revision++
}
func (a *App) Close() error {
	a.Stop()
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.wg.Wait()
	if closer, ok := a.backend.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
func (a *App) Snapshot() core.AppSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.state
	appearance := a.preferences.Snapshot()
	s.Theme = appearance.Theme
	s.Mode = appearance.Mode
	s.Screensaver = appearance.Screensaver
	s.Connectivity.Networks = append([]connectivity.Network(nil), s.Connectivity.Networks...)
	s.Connectivity.Devices = append([]connectivity.Device(nil), s.Connectivity.Devices...)
	revision := a.revision
	if a.volume != nil {
		s.Volume = a.volume.Snapshot()
		revision += s.Volume.Revision
	}
	return core.AppSnapshot{Revision: revision, Status: a.status, Text: s.Connectivity.Summary() + "\n主题: " + s.Theme + " / " + s.Mode, Data: s}
}
