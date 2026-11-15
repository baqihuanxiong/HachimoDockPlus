package newapi

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"hachimodockplus/internal/core"
	"hachimodockplus/internal/platform/controls"
)

type fakeService struct {
	fetch func(context.Context) (Dashboard, error)
	reset func(context.Context, float64, func(ResetResult)) ResetResult
}

func (f fakeService) Fetch(ctx context.Context) (Dashboard, error) { return f.fetch(ctx) }
func (f fakeService) Reset(ctx context.Context, usd float64, p func(ResetResult)) ResetResult {
	return f.reset(ctx, usd, p)
}
func testConfig() Config {
	return Config{AdminID: 1, TopN: 10, ResetUSD: 200, RefreshSeconds: 300, TimeoutSeconds: 1, MaxLogs: 50000, HealthyPercent: 95, Timezone: "Asia/Shanghai"}
}
func await(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("state transition timed out")
		}
		time.Sleep(time.Millisecond)
	}
}
func state(a *App) State { return a.Snapshot().Data.(State) }
func dispatch(t *testing.T, a *App, action string) {
	t.Helper()
	if err := a.Handle(core.Event{AppID: ID, Action: action}); err != nil {
		t.Fatal(err)
	}
}

func TestResetConfirmCancelReentryAndSnapshots(t *testing.T) {
	var writes atomic.Int32
	a := New(testConfig(), fakeService{fetch: func(context.Context) (Dashboard, error) {
		return Dashboard{Today: Usage{USD: 1}, Models: []Model{{Name: "qwen"}}, Users: []UserUsage{{Name: "alice"}}, Balances: []Balance{{Name: "MiMo"}}, Updated: time.Now()}, nil
	}, reset: func(ctx context.Context, usd float64, p func(ResetResult)) ResetResult {
		if usd != 200 {
			t.Errorf("reset amount %v", usd)
		}
		writes.Add(1)
		r := ResetResult{Total: 3, Done: 3}
		p(r)
		return r
	}})
	defer a.Close()
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return !state(a).Loading })
	s := state(a)
	s.Dashboard.Today.USD = 900
	s.Dashboard.Models[0].Name = "bad"
	s.Dashboard.Users[0].Name = "bad"
	s.Dashboard.Balances[0].Name = "bad"
	if s := state(a); s.Dashboard.Today.USD != 1 || s.Dashboard.Models[0].Name != "qwen" || s.Dashboard.Users[0].Name != "alice" || s.Dashboard.Balances[0].Name != "MiMo" {
		t.Fatal("snapshot aliases state")
	}
	dispatch(t, a, ActionConfirm)
	if writes.Load() != 0 {
		t.Fatal("unarmed confirmation wrote")
	}
	dispatch(t, a, ActionReset)
	if state(a).Countdown != 10 {
		t.Fatal(state(a))
	}
	dispatch(t, a, ActionReset)
	dispatch(t, a, HardwareAction(controls.Event{Key: controls.SW3, Pressed: true}, state(a).Countdown))
	if state(a).Countdown != 0 || writes.Load() != 0 {
		t.Fatal("cancel wrote balances")
	}
	dispatch(t, a, ActionReset)
	dispatch(t, a, HardwareAction(controls.Event{Key: controls.SW2, Pressed: true}, state(a).Countdown))
	dispatch(t, a, ActionConfirm)
	await(t, func() bool { return !state(a).Resetting })
	if writes.Load() != 1 || state(a).Result.Done != 3 {
		t.Fatal(state(a), writes.Load())
	}
	dispatch(t, a, ActionReset)
	a.Stop()
	a.Stop()
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return !state(a).Loading })
	time.Sleep(120 * time.Millisecond)
	if state(a).Countdown != 0 || writes.Load() != 1 {
		t.Fatal("reentry resumed pending reset")
	}
	if HardwareAction(controls.Event{Key: controls.SW2, Pressed: false}, 10) != "" || HardwareAction(controls.Event{Key: controls.SW3, Pressed: true}, 0) != "" {
		t.Fatal("hardware defaults consumed")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(context.Background()); !errors.Is(err, core.ErrClosed) {
		t.Fatal(err)
	}
}

func TestTenSecondCountdownAutoExecutes(t *testing.T) {
	called := make(chan time.Time, 1)
	a := New(testConfig(), fakeService{fetch: func(context.Context) (Dashboard, error) { return Dashboard{}, nil }, reset: func(context.Context, float64, func(ResetResult)) ResetResult {
		called <- time.Now()
		return ResetResult{Total: 1, Done: 1}
	}})
	defer a.Close()
	a.Start(context.Background())
	await(t, func() bool { return !state(a).Loading })
	start := time.Now()
	dispatch(t, a, ActionReset)
	select {
	case <-called:
		t.Fatal("reset executed before countdown")
	case <-time.After(1100 * time.Millisecond):
	}
	if n := state(a).Countdown; n < 8 || n > 9 {
		t.Fatal("countdown did not visibly advance", n)
	}
	select {
	case at := <-called:
		if at.Sub(start) < 10*time.Second {
			t.Fatal("countdown executed early")
		}
	case <-time.After(11 * time.Second):
		t.Fatal("countdown did not execute")
	}
}

func TestStopCancelsWorkersAndRejectsLateResults(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	a := New(testConfig(), fakeService{fetch: func(ctx context.Context) (Dashboard, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			close(cancelled)
			<-release
			return Dashboard{TotalUSD: 999}, nil
		}
		return Dashboard{TotalUSD: 3}, nil
	}})
	defer a.Close()
	a.Start(context.Background())
	<-entered
	a.Stop()
	<-cancelled
	a.Start(context.Background())
	await(t, func() bool { return !state(a).Loading })
	close(release)
	time.Sleep(30 * time.Millisecond)
	if state(a).Dashboard.TotalUSD != 3 {
		t.Fatal("late result replaced current state")
	}
	a.Stop()
	if err := a.Handle(core.Event{Action: ActionReset}); !errors.Is(err, core.ErrInactive) {
		t.Fatal(err)
	}
}

func TestFailureRetryAndResetCancellation(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{})
	exited := make(chan struct{})
	a := New(testConfig(), fakeService{fetch: func(context.Context) (Dashboard, error) {
		if calls.Add(1) == 1 {
			return Dashboard{}, errors.New("timeout")
		}
		return Dashboard{TotalUSD: 4}, nil
	}, reset: func(ctx context.Context, _ float64, _ func(ResetResult)) ResetResult {
		close(entered)
		<-ctx.Done()
		close(exited)
		return ResetResult{Error: "cancelled"}
	}})
	defer a.Close()
	a.Start(context.Background())
	await(t, func() bool { return !state(a).Loading })
	if a.Snapshot().Status != "刷新失败：timeout" {
		t.Fatal(a.Snapshot())
	}
	dispatch(t, a, ActionRefresh)
	await(t, func() bool { return !state(a).Loading })
	if state(a).Dashboard.TotalUSD != 4 {
		t.Fatal(state(a))
	}
	dispatch(t, a, ActionReset)
	dispatch(t, a, ActionConfirm)
	<-entered
	a.Stop()
	<-exited
	rev := a.Snapshot().Revision
	a.Close()
	if a.Snapshot().Revision < rev {
		t.Fatal("revision went backwards")
	}
}
