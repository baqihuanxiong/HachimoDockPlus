// Package controls translates baseboard input into debounced navigation events.
package controls

import (
	"math"
	"time"
)

type Key int

const (
	SW1 Key = iota + 1
	SW2
	SW3
	JoystickPress
	Up
	Down
	Left
	Right
)

type Event struct {
	Key     Key
	Pressed bool
}
type Options struct {
	Disabled bool `json:"disabled"`
	SwapXY   bool `json:"swap_xy"`
	InvertX  bool `json:"invert_x"`
	InvertY  bool `json:"invert_y"`
}

type debouncer struct {
	stable, pending [4]bool
	since           [4]time.Time
}

func (d *debouncer) update(values [4]bool, now time.Time) []Event {
	var events []Event
	for i, value := range values {
		if value != d.pending[i] {
			d.pending[i] = value
			d.since[i] = now
		}
		if value != d.stable[i] && now.Sub(d.since[i]) >= 20*time.Millisecond {
			d.stable[i] = value
			events = append(events, Event{Key: Key(i + 1), Pressed: value})
		}
	}
	return events
}
func normalize(raw, center float64) float64 {
	span := center
	if raw >= center {
		span = 1650 - center
	}
	return math.Max(-1, math.Min(1, (raw-center)/span))
}

type navigator struct {
	options Options
	key     Key
	next    time.Time
}

func (n *navigator) update(x, y float64, now time.Time) []Event {
	if n.options.SwapXY {
		x, y = y, x
	}
	if n.options.InvertX {
		x = -x
	}
	if n.options.InvertY {
		y = -y
	}
	threshold := 0.20
	if n.key != 0 {
		threshold = 0.15
	}
	key := Key(0)
	// Choose the dominant axis so a diagonal never skips two cards.
	if math.Max(math.Abs(x), math.Abs(y)) >= threshold {
		if math.Abs(x) >= math.Abs(y) {
			if x > 0 {
				key = Right
			} else {
				key = Left
			}
		} else if y > 0 {
			key = Down
		} else {
			key = Up
		}
	}
	if key == 0 {
		n.key = 0
		return nil
	}
	if key != n.key {
		n.key = key
		n.next = now.Add(400 * time.Millisecond)
		return []Event{{Key: key, Pressed: true}}
	}
	if !now.Before(n.next) {
		n.next = now.Add(150 * time.Millisecond)
		return []Event{{Key: key, Pressed: true}}
	}
	return nil
}
