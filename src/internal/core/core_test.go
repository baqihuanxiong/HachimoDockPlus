package core

import (
	"context"
	"errors"
	"testing"
)

type testApp struct {
	ctx                   context.Context
	starts, stops, closes int
	events                []Event
	fail                  bool
}

func (a *testApp) Start(ctx context.Context) error {
	a.ctx = ctx
	a.starts++
	if a.fail {
		return errors.New("failed")
	}
	return nil
}
func (a *testApp) Stop()                 { a.stops++ }
func (a *testApp) Handle(e Event) error  { a.events = append(a.events, e); return nil }
func (a *testApp) Snapshot() AppSnapshot { return AppSnapshot{Status: "ready"} }
func (a *testApp) Close() error          { a.closes++; return nil }
func TestRegistryValidationAndMetadataOwnership(t *testing.T) {
	caps := []Capability{Network}
	e := Registration{Manifest: Manifest{ID: "one", Title: "One", Capabilities: caps}, New: func() (Application, error) { return &testApp{}, nil }}
	r, err := NewRegistry(e)
	if err != nil {
		t.Fatal(err)
	}
	caps[0] = Speaker
	m := r.Manifests()
	m[0].Capabilities[0] = Microphone
	if r.Manifests()[0].Capabilities[0] != Network {
		t.Fatal("mutable metadata escaped")
	}
	if _, err = NewRegistry(e, e); err == nil {
		t.Fatal("duplicate accepted")
	}
	e.Manifest.ID = "../escape"
	if _, err = NewRegistry(e); err == nil {
		t.Fatal("invalid ID accepted")
	}
}
func TestHostLazyFactoriesLifecycleAndStaleEvents(t *testing.T) {
	a, b := &testApp{}, &testApp{}
	created := 0
	r, _ := NewRegistry(
		Registration{Manifest: Manifest{ID: "one", Title: "One"}, New: func() (Application, error) { created++; return a, nil }},
		Registration{Manifest: Manifest{ID: "two", Title: "Two"}, New: func() (Application, error) { created++; return b, nil }},
	)
	h := NewHost(context.Background(), r)
	if created != 0 || h.Snapshot().Active != "" {
		t.Fatal("apps initialized before open")
	}
	if err := h.Open("one"); err != nil {
		t.Fatal(err)
	}
	oldCtx := a.ctx
	if err := h.Dispatch(Event{AppID: "one", Action: "refresh"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Open("two"); err != nil {
		t.Fatal(err)
	}
	if oldCtx.Err() != context.Canceled || a.stops != 1 {
		t.Fatal("old app not stopped/cancelled")
	}
	if err := h.Dispatch(Event{AppID: "one", Action: "late"}); !errors.Is(err, ErrStaleEvent) {
		t.Fatal(err)
	}
	if len(b.events) != 0 {
		t.Fatal("stale event reached another app")
	}
	h.Back()
	if b.ctx.Err() != context.Canceled || h.Snapshot().Active != "" {
		t.Fatal("back retained foreground work")
	}
	if err := h.Open("one"); err != nil {
		t.Fatal(err)
	}
	if created != 2 || a.starts != 2 || a.ctx == oldCtx {
		t.Fatal("re-entry did not reuse instance with fresh context")
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	_ = h.Close()
	if a.closes != 1 || b.closes != 1 {
		t.Fatal("instances closed incorrectly")
	}
	if err := h.Open("one"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
func TestFailedStartReturnsToCatalogAndReleasesInstance(t *testing.T) {
	a := &testApp{fail: true}
	r, _ := NewRegistry(Registration{Manifest: Manifest{ID: "bad", Title: "Bad"}, New: func() (Application, error) { return a, nil }})
	h := NewHost(context.Background(), r)
	defer h.Close()
	if err := h.Open("bad"); err == nil {
		t.Fatal("failed start accepted")
	}
	s := h.Snapshot()
	if s.Active != "" || s.Status == "" || a.ctx.Err() != context.Canceled || a.stops != 1 || a.closes != 1 {
		t.Fatal("failed start leaked resources", s)
	}
	if err := h.Dispatch(Event{AppID: "bad", Action: "go"}); !errors.Is(err, ErrInactive) {
		t.Fatal(err)
	}
}
