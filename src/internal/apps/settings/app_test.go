package settings

import (
	"context"
	"encoding/json"
	"errors"
	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/audio"
	"hachimodockplus/internal/platform/connectivity"
	"hachimodockplus/internal/platform/system"
	"hachimodockplus/internal/preferences"
	"path/filepath"
	"strings"
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

func TestScreensaverEventPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	prefs, _ := preferences.Load(path)
	a := New(&fakeBackend{}, prefs)
	defer a.Close()
	a.Start(context.Background())
	ready(t, a)
	for _, id := range []string{"blank", "clock", "unknown"} {
		if err := a.Handle(core.Event{Action: "screensaver.set", Text: id}); err != nil {
			t.Fatal(err)
		}
		ready(t, a)
		state := a.Snapshot().Data.(State)
		if id == "unknown" {
			if state.Screensaver != "clock" || a.Snapshot().Status == "操作完成" {
				t.Fatal(a.Snapshot())
			}
		} else if state.Screensaver != id {
			t.Fatal(state)
		}
	}
	loaded, err := preferences.Load(path)
	if err != nil || loaded.Snapshot().Screensaver != "clock" {
		t.Fatal(loaded, err)
	}
}

type volumeMixer struct{}

func (volumeMixer) Apply(context.Context, int) error { return nil }
func TestAudioVolumeRemainsAvailableDuringNetworkTask(t *testing.T) {
	v, _ := audio.NewVolume("", volumeMixer{})
	defer v.Close()
	b := &fakeBackend{block: make(chan struct{}), started: make(chan struct{}, 1)}
	prefs, _ := preferences.Load("")
	a := New(b, prefs, v)
	defer a.Close()
	_ = a.Start(context.Background())
	ready(t, a)
	if err := a.Handle(core.Event{Action: "wifi.scan"}); err != nil {
		t.Fatal(err)
	}
	<-b.started
	if err := a.Handle(core.Event{Action: "volume.set", Text: "0"}); err != nil {
		t.Fatal(err)
	}
	if s := a.Snapshot().Data.(State); s.Volume.Volume != 0 || !s.Busy {
		t.Fatal(s)
	}
	v.Set(75)
	if a.Snapshot().Data.(State).Volume.Volume != 75 {
		t.Fatal("shared adjustment not reflected")
	}
}

type fakeSystem struct {
	mu       sync.Mutex
	state    system.State
	restarts int
}

func (s *fakeSystem) Read(context.Context) system.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}
func (s *fakeSystem) ConfigureSMB(_ context.Context, c system.Credentials) error {
	if err := system.ValidateCredentials(c); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Username = c.Username
	s.state.PasswordSet = true
	s.state.Enabled = true
	return nil
}
func (s *fakeSystem) SetSMB(_ context.Context, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Enabled = on
	return nil
}
func (s *fakeSystem) Restart(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restarts++
	return nil
}
func TestSystemActionsNeverExposePasswordInSnapshots(t *testing.T) {
	prefs, _ := preferences.Load("")
	sys := &fakeSystem{state: system.State{Available: true, CanRestart: true}}
	a := NewWithSystem(&fakeBackend{}, prefs, nil, sys)
	defer a.Close()
	a.Start(context.Background())
	ready(t, a)
	password := "shared-secret-never-display"
	for _, e := range []core.Event{{Action: "smb.account", Text: `{"Username":"docktest","Password":"` + password + `"}`}, {Action: "smb.set", Text: "false"}, {Action: "system.restart"}} {
		if err := a.Handle(e); err != nil {
			t.Fatal(err)
		}
		ready(t, a)
		serialized, _ := json.Marshal(a.Snapshot())
		if strings.Contains(string(serialized), password) {
			t.Fatal("password in snapshot")
		}
	}
	s := a.Snapshot().Data.(State).System
	if s.Username != "docktest" || !s.PasswordSet || s.Enabled || sys.restarts != 1 {
		t.Fatal(s, sys.restarts)
	}
}

type audioBackend struct{ fakeBackend }

func (*audioBackend) Read(context.Context) connectivity.State {
	return connectivity.State{Devices: []connectivity.Device{
		{Address: "11:22:33:44:55:66", Name: "Headphones", Paired: true, Audio: true, Available: true},
		{Address: "AA:BB:CC:DD:EE:FF", Name: "Keyboard", Paired: true, Available: true},
	}}
}
func TestOutputSelectionIsSharedAndDoesNotRestart(t *testing.T) {
	prefs, _ := preferences.Load("")
	sys := &fakeSystem{}
	v, err := audio.NewConfiguredVolume(audio.VolumeState{Volume: 37, Output: "speaker"}, audio.OutputDevices{Speaker: "analog", SPDIF: "digital"}, volumeMixer{}, func(audio.VolumeState) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	a := NewWithSystem(&audioBackend{}, prefs, v, sys)
	defer a.Close()
	a.Start(context.Background())
	ready(t, a)
	for _, e := range []core.Event{
		{Action: "audio.output", Text: "spdif"},
		{Action: "audio.bluetooth", Text: "11:22:33:44:55:66"},
	} {
		if err = a.Handle(e); err != nil {
			t.Fatal(err)
		}
		ready(t, a)
	}
	st := a.Snapshot().Data.(State)
	if st.Volume.Output != "bluetooth" || st.Volume.BluetoothDevice != "11:22:33:44:55:66" || st.Volume.Volume != 37 || sys.restarts != 0 {
		t.Fatal(st.Volume, sys.restarts)
	}
	for _, e := range []core.Event{
		{Action: "audio.bluetooth", Text: "AA:BB:CC:DD:EE:FF"},
		{Action: "audio.bluetooth", Text: "00:00:00:00:00:00"},
		{Action: "audio.output", Text: "invalid"},
	} {
		if err = a.Handle(e); err != nil {
			t.Fatal(err)
		}
		ready(t, a)
		if a.Snapshot().Status == "操作完成" || v.Snapshot().Output != "bluetooth" {
			t.Fatal("invalid output changed route", a.Snapshot())
		}
	}
}

type discoveryBackend struct {
	fakeBackend
	stopped int
	closed  bool
}

func (b *discoveryBackend) StopDiscovery() { b.mu.Lock(); b.stopped++; b.mu.Unlock() }
func (b *discoveryBackend) Close() error   { b.mu.Lock(); b.closed = true; b.mu.Unlock(); return nil }
func TestSettingsOwnsDiscoveryLifecycle(t *testing.T) {
	prefs, _ := preferences.Load("")
	b := &discoveryBackend{}
	a := New(b, prefs)
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ready(t, a)
	a.Stop()
	b.mu.Lock()
	stopped := b.stopped
	b.mu.Unlock()
	if stopped != 1 {
		t.Fatal("leaving settings did not cancel discovery")
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ready(t, a)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped != 2 || !b.closed {
		t.Fatal("closing settings did not join discovery", b.stopped, b.closed)
	}
}
