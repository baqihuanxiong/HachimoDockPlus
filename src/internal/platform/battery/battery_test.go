package battery

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
	"unsafe"
)

type fakeSensor struct {
	values       map[byte]uint16
	failRegister byte
	fail         bool
	writes       []byte
}

func sensor() *fakeSensor {
	return &fakeSensor{values: map[byte]uint16{0xfe: 0x5449, 0xff: 0x2260, 0: 0x4127, 5: 0, 6: 8, 2: 3340, 1: 0xf670, 4: 0xf670}}
}
func (d *fakeSensor) read(r byte) (uint16, error) { return d.values[r], nil }
func (d *fakeSensor) write(r byte, v uint16) error {
	d.writes = append(d.writes, r)
	if d.fail && r == d.failRegister {
		d.fail = false
		return errors.New("injected I2C failure")
	}
	d.values[r] = v
	return nil
}
func TestMeasurementAndRestoration(t *testing.T) {
	d := sensor()
	s, err := measure(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(s.Voltage-4.16888) > .00001 || s.CurrentMA != -612 || !s.Charging() {
		t.Fatal(s)
	}
	if d.values[0] != 0x4127 || d.values[5] != 0 {
		t.Fatal("sensor configuration not restored", d.values)
	}
}
func TestFailuresRestoreSensor(t *testing.T) {
	for _, kind := range []string{"overflow", "canceled", "write"} {
		t.Run(kind, func(t *testing.T) {
			d := sensor()
			ctx := context.Background()
			switch kind {
			case "overflow":
				d.values[6] = 12
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "write":
				d.fail = true
				d.failRegister = 5
			}
			if _, err := measure(ctx, d); err == nil {
				t.Fatal("failure accepted")
			}
			if d.values[0] != 0x4127 || d.values[5] != 0 {
				t.Fatal("failed to restore", d.values)
			}
		})
	}
}
func TestDecodeValidation(t *testing.T) {
	for _, tc := range []struct{ bus, shunt, current uint16 }{{0x8000, 0, 0}, {3300, 32767, 32767}, {3300, 3, 7}} {
		if _, err := decode(tc.bus, tc.shunt, tc.current); err == nil {
			t.Fatal(tc)
		}
	}
	s, err := decode(3200, 1000, 1000)
	if err != nil || s.CurrentMA != 250 || s.Direction() != "放电中" {
		t.Fatal(s, err)
	}
	d := sensor()
	d.values[0xfe] = 0
	if _, err = measure(context.Background(), d); err == nil || len(d.writes) > 0 {
		t.Fatal("wrote to unknown device")
	}
}
func TestCanceledReadDoesNotOpenDevice(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := New()
	r.Path = "/nonexistent"
	start := time.Now()
	_, err := r.Read(ctx)
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatal(err)
	}
}
func TestI2CABI(t *testing.T) {
	var msg i2cMessage
	var transfer i2cTransfer
	if unsafe.Offsetof(msg.buffer) != 8 || unsafe.Offsetof(transfer.count) != unsafe.Sizeof(uintptr(0)) {
		t.Fatal("incorrect Linux I2C ABI")
	}
}
