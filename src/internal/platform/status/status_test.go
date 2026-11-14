package status

import (
	"context"
	"hachimodockplus/internal/platform/battery"
	"hachimodockplus/internal/platform/connectivity"
	"testing"
	"time"
)

type batteryFunc func(context.Context) (battery.Sample, error)

func (f batteryFunc) Read(ctx context.Context) (battery.Sample, error) { return f(ctx) }

type networkFunc func(context.Context) connectivity.Indicators

func (f networkFunc) Indicators(ctx context.Context) connectivity.Indicators { return f(ctx) }
func TestClockTimezoneAndUnsynchronizedTime(t *testing.T) {
	m, err := New(nil, nil, "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	clock := m.Clock(time.Date(2026, 11, 14, 18, 5, 9, 0, time.UTC))
	if clock != "11-15 Sun 02:05" {
		t.Fatal(clock)
	}
	clock = m.Clock(time.Unix(0, 0))
	if clock != "等待对时" {
		t.Fatal(clock)
	}
	if _, err = New(nil, nil, "not-a-timezone"); err == nil {
		t.Fatal("invalid timezone accepted")
	}
}
func TestIndependentPollingAndShutdown(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	m, err := New(batteryFunc(func(context.Context) (battery.Sample, error) {
		return battery.Sample{Voltage: 4.17, CurrentMA: -600}, nil
	}), networkFunc(func(ctx context.Context) connectivity.Indicators {
		close(started)
		<-ctx.Done()
		close(stopped)
		return connectivity.Indicators{}
	}), "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	stop := m.Start(context.Background())
	defer stop()
	<-started
	deadline := time.Now().Add(time.Second)
	for !m.Snapshot().BatteryReady && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s := m.Snapshot(); !s.BatteryReady || s.Battery.Voltage != 4.17 {
		t.Fatal(s)
	}
	stop()
	select {
	case <-stopped:
	default:
		t.Fatal("monitor returned before worker stopped")
	}
}
