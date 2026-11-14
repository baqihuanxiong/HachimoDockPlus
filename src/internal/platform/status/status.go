// Package status polls shared device indicators independently of foreground apps.
package status

import (
	"context"
	"hachimodockplus/internal/platform/battery"
	"hachimodockplus/internal/platform/connectivity"
	"sync"
	"time"
	_ "time/tzdata"
)

type Battery interface {
	Read(context.Context) (battery.Sample, error)
}
type Network interface {
	Indicators(context.Context) connectivity.Indicators
}
type Snapshot struct {
	Battery      battery.Sample
	BatteryReady bool
	Radios       connectivity.Indicators
}
type Monitor struct {
	mu       sync.Mutex
	snapshot Snapshot
	battery  Battery
	network  Network
	Location *time.Location
}

func New(b Battery, n Network, zone string) (*Monitor, error) {
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, err
	}
	return &Monitor{battery: b, network: n, Location: location}, nil
}
func (m *Monitor) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshot }

// Start returns a stop function that cancels and joins both workers.
func (m *Monitor) Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	var wg sync.WaitGroup
	poll := func(interval time.Duration, read func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				sample, c := context.WithTimeout(ctx, 4*time.Second)
				read(sample)
				c()
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
	poll(3*time.Second, func(ctx context.Context) {
		sample, err := m.battery.Read(ctx)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.snapshot.Battery = sample
		m.snapshot.BatteryReady = err == nil
	})
	poll(5*time.Second, func(ctx context.Context) {
		radios := m.network.Indicators(ctx)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.snapshot.Radios = radios
	})
	return func() { cancel(); wg.Wait() }
}

// Clock hides the invalid 1970 boot date until background SNTP succeeds.
func (m *Monitor) Clock(now time.Time) string {
	if now.Year() < 2024 {
		return "等待对时"
	}
	return now.In(m.Location).Format("01-02 Mon 15:04")
}
