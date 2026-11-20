package pet

import (
	"context"
	"path/filepath"
	"time"

	"hachimodockplus/internal/platform/desktoplan"
)

// Companion owns the always-available LAN sink; video decoding is separately
// owned by the visible screensaver and stops when that screen is dismissed.
type Companion struct {
	Tracker     Tracker
	Pack        Pack
	MediaError  error
	LAN         *desktoplan.Server
	WiFiEnabled bool
}

func New(directory, dataDir string, wifi bool) *Companion {
	c := &Companion{WiFiEnabled: wifi}
	c.Pack, c.MediaError = LoadPack(directory)
	c.LAN = desktoplan.New(desktoplan.Options{Path: filepath.Join(dataDir, "pet", "lan.json"), Apply: c.Tracker.Apply, Probe: c.Tracker.Probe, Clear: c.Tracker.Clear})
	return c
}
func (c *Companion) Start(ctx context.Context) error {
	if c.WiFiEnabled {
		return c.LAN.Start(ctx)
	}
	return nil
}
func (c *Companion) Close()        { c.LAN.Close() }
func (c *Companion) Forget() error { return c.LAN.Forget() }
func (c *Companion) PairStatus() (string, bool) {
	state := c.LAN.State()
	if state.Error != "" {
		return state.Error, state.Paired
	}
	if !c.WiFiEnabled {
		return "Desktop Wi-Fi 未启用", false
	}
	if !state.Running {
		return "Desktop Wi-Fi 未启动", state.Paired
	}
	if !state.Paired {
		return "等待 Desktop 配对", false
	}
	if c.Tracker.View(time.Now()).Connected {
		return "Desktop 已连接", true
	}
	return "已配对 · 等待 Desktop", true
}
