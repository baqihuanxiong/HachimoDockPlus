package settings

import (
	"context"
	"errors"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/connectivity"
	"hachimodockplus/internal/preferences"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	mu      sync.Mutex
	calls   int
	block   chan struct{}
	started chan struct{}
	err     error
}

func (b *fakeBackend) Read(context.Context) connectivity.State {
	return connectivity.State{Networks: []connectivity.Network{{SSID: "original"}}, Devices: []connectivity.Device{{Name: "device"}}}
}
func (b *fakeBackend) ScanWiFi(ctx context.Context) error {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	if b.started != nil {
		b.started <- struct{}{}
	}
	if b.block != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.block:
		}
	}
	return b.err
}
func (b *fakeBackend) JoinWiFi(context.Context, connectivity.Join) error { return b.err }
func (b *fakeBackend) ConnectSaved(context.Context, string) error        { return b.err }
func (b *fakeBackend) DisconnectWiFi(context.Context) error              { return b.err }
func (b *fakeBackend) ForgetWiFi(context.Context, string) error          { return b.err }
func (b *fakeBackend) Bluetooth(context.Context, string, string) error   { return b.err }
func ready(t *testing.T, a *App) {
	t.Helper()
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		if !a.Snapshot().Data.(State).Busy {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("app remained busy")
}
func TestSettingsLifecycleAndSnapshots(t *testing.T) {
	b := &fakeBackend{block: make(chan struct{}), started: make(chan struct{}, 1)}
	prefs, _ := preferences.Load("")
	a := New(b, prefs)
	defer a.Close()
	if a.Handle(core.Event{Action: "refresh"}) != core.ErrInactive {
		t.Fatal("inactive accepted")
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ready(t, a)
	s := a.Snapshot().Data.(State)
	s.Connectivity.Networks[0].SSID = "changed"
	s.Connectivity.Devices[0].Name = "changed"
	if a.Snapshot().Data.(State).Connectivity.Networks[0].SSID != "original" || a.Snapshot().Data.(State).Connectivity.Devices[0].Name != "device" {
		t.Fatal("mutable snapshot")
	}
	if err := a.Handle(core.Event{Action: "wifi.scan"}); err != nil {
		t.Fatal(err)
	}
	<-b.started
	if a.Handle(core.Event{Action: "wifi.scan"}) == nil {
		t.Fatal("concurrent task accepted")
	}
	a.Stop()
	a.Start(context.Background())
	ready(t, a)
	if a.Snapshot().Status != "准备就绪" {
		t.Fatal("late canceled result overwrote new entry")
	}
	a.Close()
	a.Close()
	if a.Start(context.Background()) != core.ErrClosed {
		t.Fatal("closed app started")
	}
}
func TestSettingsOperationFailure(t *testing.T) {
	b := &fakeBackend{err: errors.New("test failure")}
	prefs, _ := preferences.Load("")
	a := New(b, prefs)
	defer a.Close()
	a.Start(context.Background())
	ready(t, a)
	if a.Handle(core.Event{Action: "bt.on"}) != nil {
		t.Fatal("dispatch failed")
	}
	ready(t, a)
	if a.Snapshot().Status != "test failure" {
		t.Fatal(a.Snapshot())
	}
	if a.Handle(core.Event{Action: "unknown"}) != core.ErrUnknownAction {
		t.Fatal("unknown action accepted")
	}
}

func TestThemeChangePersistsAndUpdatesSnapshot(t *testing.T) {
	prefs, err := preferences.Load(filepath.Join(t.TempDir(), "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(&fakeBackend{}, prefs)
	defer a.Close()
	a.Start(context.Background())
	ready(t, a)
	if err = a.Handle(core.Event{Action: "theme.set", Text: "violet"}); err != nil {
		t.Fatal(err)
	}
	ready(t, a)
	if a.Snapshot().Data.(State).Theme != "violet" || a.Snapshot().Status != "操作完成" {
		t.Fatal(a.Snapshot())
	}
	if err = a.Handle(core.Event{Action: "theme.set", Text: "invalid"}); err != nil {
		t.Fatal(err)
	}
	ready(t, a)
	if a.Snapshot().Data.(State).Theme != "violet" || a.Snapshot().Status == "操作完成" {
		t.Fatal("invalid theme accepted")
	}
}

func TestModeChangesPreserveTheme(t *testing.T) {
	prefs, _ := preferences.Load("")
	prefs.SetTheme("amber")
	a := New(&fakeBackend{}, prefs)
	defer a.Close()
	a.Start(context.Background())
	ready(t, a)
	if err := a.Handle(core.Event{Action: "mode.set", Text: "light"}); err != nil {
		t.Fatal(err)
	}
	ready(t, a)
	state := a.Snapshot().Data.(State)
	if state.Mode != "light" || state.Theme != "amber" {
		t.Fatal(state)
	}
	if err := a.Handle(core.Event{Action: "theme.set", Text: "orange"}); err != nil {
		t.Fatal(err)
	}
	ready(t, a)
	state = a.Snapshot().Data.(State)
	if state.Mode != "light" || state.Theme != "orange" {
		t.Fatal(state)
	}
}
