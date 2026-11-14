// Package battery reads the baseboard INA226 without estimating uncalibrated state of charge.
package battery

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const calibration uint16 = 0x0800

// Sample uses the schematic's 10 mOhm shunt; positive current means discharge.
type Sample struct{ Voltage, CurrentMA float64 }

func (s Sample) Charging() bool { return s.CurrentMA < -5 }
func (s Sample) Direction() string {
	if s.Charging() {
		return "充电中"
	}
	if s.CurrentMA > 5 {
		return "放电中"
	}
	return "空闲"
}

type Reader struct {
	Path    string
	Address uint16
}

func New() *Reader { return &Reader{Path: "/dev/i2c-2", Address: 0x40} }

type registers interface {
	read(byte) (uint16, error)
	write(byte, uint16) error
}

func decode(bus, shunt, current uint16) (Sample, error) {
	s, c := int16(shunt), int16(current)
	if bus&0x8000 != 0 || s >= 32760 || s <= -32760 || int(s)-int(c) > 1 || int(c)-int(s) > 1 {
		return Sample{}, errors.New("电池采样数据无效")
	}
	return Sample{Voltage: float64(bus)*.00125 + float64(s)*.0000025, CurrentMA: float64(c) * .25}, nil
}
func measure(ctx context.Context, d registers) (sample Sample, err error) {
	manufacturer, err := d.read(0xfe)
	if err != nil {
		return sample, err
	}
	die, err := d.read(0xff)
	if err != nil {
		return sample, err
	}
	if manufacturer != 0x5449 || die&0xfff0 != 0x2260 {
		return sample, errors.New("未检测到 INA226")
	}
	config, err := d.read(0)
	if err != nil {
		return sample, err
	}
	cal, err := d.read(5)
	if err != nil {
		return sample, err
	}
	// Restore both registers on every exit, including configuration failure/cancel.
	defer func() {
		e1 := d.write(5, cal)
		e2 := d.write(0, config&0x7fff)
		r1, e3 := d.read(5)
		r2, e4 := d.read(0)
		err = errors.Join(err, e1, e2, e3, e4)
		if e3 == nil && e4 == nil && (r1 != cal || r2 != config) {
			err = errors.Join(err, errors.New("电池采样配置恢复失败"))
		}
	}()
	if err = d.write(0, 0x4520); err != nil {
		return sample, err
	}
	if err = d.write(5, calibration); err != nil {
		return sample, err
	}
	if err = d.write(0, 0x4523); err != nil {
		return sample, err
	}
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return sample, ctx.Err()
		case <-timer.C:
			return sample, errors.New("电池采样超时")
		case <-tick.C:
			mask, e := d.read(6)
			if e != nil {
				return sample, e
			}
			if mask&4 != 0 {
				return sample, errors.New("电池采样溢出")
			}
			if mask&8 == 0 {
				continue
			}
			actual, e := d.read(5)
			if e != nil {
				return sample, e
			}
			if actual != calibration {
				return sample, errors.New("电池校准寄存器已变化")
			}
			bus, e := d.read(2)
			if e != nil {
				return sample, e
			}
			shunt, e := d.read(1)
			if e != nil {
				return sample, e
			}
			current, e := d.read(4)
			if e != nil {
				return sample, e
			}
			return decode(bus, shunt, current)
		}
	}
}
func (s Sample) String() string { return fmt.Sprintf("%.2fV %s", s.Voltage, s.Direction()) }
