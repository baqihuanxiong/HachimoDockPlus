package core

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type testApp struct {
	ctx                   context.Context
	starts, stops, closes int
	events                []Event
	fail                  bool
}

func TestRegistryOrderingKeepsUnlistedAppsAndFactoryIDs(t *testing.T) {
	created := 0
	apps := map[string]*testApp{"one": {}, "two": {}, "three": {}}
	var entries []Registration
	for _, id := range []string{"one", "two", "three"} {
		entries = append(entries, Registration{Manifest: Manifest{ID: id, Title: id}, New: func() (Application, error) {
			created++
			return apps[id], nil
		}})
	}
	r, err := NewRegistry(entries...)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ order, want []string }{
		{nil, []string{"one", "two", "three"}},
		{[]string{}, []string{"one", "two", "three"}},
		{[]string{"three"}, []string{"three", "one", "two"}},
		{[]string{"two", "three", "one"}, []string{"two", "three", "one"}},
	} {
		ordered, err := r.Ordered(tc.order)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, m := range ordered.Manifests() {
			got = append(got, m.ID)
		}
		if !reflect.DeepEqual(got, tc.want) || created != 0 || r.Manifests()[0].ID != "one" {
			t.Fatal("ordering changed factories or original registry", got)
		}
	}
	for _, order := range [][]string{{"missing"}, {"one", "one"}, {""}} {
		if _, err := r.Ordered(order); err == nil {
			t.Fatal("invalid order accepted", order)
		}
	}
	ordered, _ := r.Ordered([]string{"three", "two", "one"})
	h := NewHost(context.Background(), ordered)
	defer h.Close()
	if err := h.Open("three"); err != nil || apps["three"].starts != 1 || apps["one"].starts != 0 {
		t.Fatal("opening a reordered ID used the wrong factory", err)
	}
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

type backgroundApp struct {
	testApp
	running bool
}

func (a *backgroundApp) BackgroundRunning() bool { return a.running }
func TestCatalogReportsLiveBackgroundWork(t *testing.T) {
	a := &backgroundApp{}
	r, _ := NewRegistry(Registration{Manifest: Manifest{ID: "worker", Title: "Worker"}, New: func() (Application, error) { return a, nil }})
	h := NewHost(context.Background(), r)
	defer h.Close()
	h.Open("worker")
	if !h.Snapshot().Running["worker"] {
		t.Fatal("foreground app missing")
	}
	h.Back()
	if h.Snapshot().Running["worker"] {
		t.Fatal("cached instance marked running")
	}
	a.running = true
	s := h.Snapshot()
	if !s.Running["worker"] {
		t.Fatal("background state missing")
	}
	s.Running["worker"] = false
	if !h.Snapshot().Running["worker"] {
		t.Fatal("mutable status map escaped")
	}
	a.running = false
	if h.Snapshot().Running["worker"] {
		t.Fatal("stale running state")
	}
}
