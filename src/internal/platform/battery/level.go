package battery

import "math"

// ApproximateLevel is a display-only estimate for a standard 4.2 V single Li-ion
// cell. This generic curve is not a calibrated fuel gauge: load, charging and
// temperature affect it. Never use it for battery protection or charge control.
func (s Sample) ApproximateLevel() int {
	if math.IsNaN(s.Voltage) || math.IsInf(s.Voltage, 0) || s.Voltage < 2.5 || s.Voltage > 4.5 {
		return -1
	}
	curve := []struct{ v, level float64 }{{3.4, 0}, {3.5, 5}, {3.6, 10}, {3.7, 20}, {3.8, 40}, {3.9, 60}, {4.0, 80}, {4.1, 90}, {4.2, 100}}
	if s.Voltage <= curve[0].v {
		return 0
	}
	for i := 1; i < len(curve); i++ {
		a, b := curve[i-1], curve[i]
		if s.Voltage <= b.v {
			return int(math.Round(a.level + (s.Voltage-a.v)/(b.v-a.v)*(b.level-a.level)))
		}
	}
	return 100
}
