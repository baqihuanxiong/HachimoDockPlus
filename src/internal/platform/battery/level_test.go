package battery

import (
	"math"
	"testing"
)

func TestApproximateLevel(t *testing.T) {
	for _, tc := range []struct {
		v     float64
		level int
	}{{2.4, -1}, {3.3, 0}, {3.5, 5}, {3.6, 10}, {3.7, 20}, {3.8, 40}, {3.9, 60}, {4.0, 80}, {4.2, 100}, {4.3, 100}, {4.6, -1}, {math.NaN(), -1}, {math.Inf(1), -1}} {
		if got := (Sample{Voltage: tc.v}).ApproximateLevel(); got != tc.level {
			t.Errorf("%.2f V: got %d want %d", tc.v, got, tc.level)
		}
	}
	previous := -1
	for mv := 3000; mv <= 4300; mv++ {
		level := (Sample{Voltage: float64(mv) / 1000}).ApproximateLevel()
		if level < previous {
			t.Fatal("non-monotonic estimate", mv, level)
		}
		previous = level
	}
}
