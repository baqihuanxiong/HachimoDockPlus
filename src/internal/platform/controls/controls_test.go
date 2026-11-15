package controls

import (
	"testing"
	"time"
	"unsafe"
)

func TestDebounceAndSimultaneousButtons(t *testing.T) {
	now := time.Now()
	d := debouncer{}
	pressed := [4]bool{false, true, true, false}
	if len(d.update(pressed, now)) != 0 {
		t.Fatal("not debounced")
	}
	if len(d.update([4]bool{}, now.Add(10*time.Millisecond))) != 0 {
		t.Fatal("bounce")
	}
	d.update(pressed, now.Add(15*time.Millisecond))
	got := d.update(pressed, now.Add(36*time.Millisecond))
	if len(got) != 2 || got[0] != (Event{SW2, true}) || got[1] != (Event{SW3, true}) {
		t.Fatal(got)
	}
	d.update([4]bool{}, now.Add(40*time.Millisecond))
	got = d.update([4]bool{}, now.Add(61*time.Millisecond))
	if len(got) != 2 || got[0].Pressed || got[1].Pressed {
		t.Fatal(got)
	}
	if unsafe.Sizeof(gpioRequest{}) != 364 {
		t.Fatal("GPIO ABI size")
	}
}
func TestJoystickDeadzoneRepeatAndMapping(t *testing.T) {
	now := time.Now()
	n := navigator{}
	if len(n.update(.19, 0, now)) != 0 {
		t.Fatal("deadzone")
	}
	check := func(x, y float64, dt time.Duration, key Key) {
		t.Helper()
		got := n.update(x, y, now.Add(dt))
		if key == 0 {
			if len(got) != 0 {
				t.Fatal(got)
			}
		} else if len(got) != 1 || got[0].Key != key {
			t.Fatal(got, key)
		}
	}
	check(.7, .3, 0, Right)
	check(.18, 0, 100*time.Millisecond, 0)
	check(.7, 0, 399*time.Millisecond, 0)
	check(.7, 0, 400*time.Millisecond, Right)
	check(.7, 0, 550*time.Millisecond, Right)
	check(0, -.8, 560*time.Millisecond, Up)
	check(0, 0, 570*time.Millisecond, 0)
	check(0, -.8, 580*time.Millisecond, Up)
	n = navigator{options: Options{SwapXY: true, InvertY: true}}
	check(1, 0, time.Second, Up)
	n = navigator{options: Options{InvertY: true}}
	check(0, .8, 2*time.Second, Up)
	check(0, -.8, 3*time.Second, Down)
	check(.8, 0, 4*time.Second, Right)
	check(-.8, 0, 5*time.Second, Left)
	if normalize(0, 861) != -1 || normalize(1650, 861) != 1 || normalize(861, 861) != 0 {
		t.Fatal("normalization")
	}
}
