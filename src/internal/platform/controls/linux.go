//go:build linux

package controls

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

//go:uintptrescapes
func ioctl(fd uintptr, request, arg uintptr) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, request, arg)
	if errno != 0 {
		return errno
	}
	return nil
}

// Start keeps slow I2C conversions off the UI thread. The two workers fail
// independently: an unavailable joystick does not disable SW2/SW3 or touch.
func Start(parent context.Context, options Options) (<-chan Event, func()) {
	ctx, cancel := context.WithCancel(parent)
	events := make(chan Event, 32)
	var wg sync.WaitGroup
	emit := func(e Event) {
		select {
		case events <- e:
		case <-ctx.Done():
		}
	}
	for _, worker := range []func(context.Context, func(Event)) error{buttons, func(ctx context.Context, emit func(Event)) error { return joystick(ctx, options, emit) }} {
		wg.Add(1)
		go func(run func(context.Context, func(Event)) error) {
			defer wg.Done()
			if err := run(ctx, emit); err != nil && ctx.Err() == nil {
				log.Printf("物理输入不可用: %v", err)
			}
		}(worker)
	}
	return events, func() { cancel(); wg.Wait() }
}

type gpioRequest struct {
	Offsets  [64]uint32
	Flags    uint32
	Defaults [64]byte
	Consumer [32]byte
	Lines    uint32
	FD       int32
}

func buttons(ctx context.Context, emit func(Event)) error {
	chip, err := os.Open("/dev/gpiochip0")
	if err != nil {
		return err
	}
	defer chip.Close()
	var info [68]byte
	if err = ioctl(chip.Fd(), 0x8044b401, uintptr(unsafe.Pointer(&info))); err != nil {
		return err
	}
	if string(bytes.TrimRight(info[32:64], "\x00")) != "gpio0" {
		return errors.New("需要 gpio0")
	}
	request := gpioRequest{Offsets: [64]uint32{2, 3, 4, 5}, Flags: 1 | 4 | 32, Lines: 4, FD: -1}
	copy(request.Consumer[:], "hachimodock-controls")
	if err = ioctl(chip.Fd(), 0xc000b403|unsafe.Sizeof(request)<<16, uintptr(unsafe.Pointer(&request))); err != nil {
		return err
	}
	handle := os.NewFile(uintptr(request.FD), "controls-gpio")
	defer handle.Close()
	restore, err := pullups()
	if err != nil {
		return err
	}
	defer restore()
	read := func() ([4]bool, error) {
		var data [64]byte
		err := ioctl(handle.Fd(), 0xc040b408, uintptr(unsafe.Pointer(&data)))
		return [4]bool{data[0] != 0, data[1] != 0, data[2] != 0, data[3] != 0}, err
	}
	values, err := read()
	if err != nil {
		return err
	}
	keys := debouncer{stable: values, pending: values}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			values, err = read()
			if err != nil {
				return err
			}
			for _, e := range keys.update(values, now) {
				emit(e)
			}
		}
	}
}
func pullups() (func(), error) {
	const node = "/sys/firmware/devicetree/base/syscon@ff950000/"
	compatible, err := os.ReadFile(node + "compatible")
	if err != nil {
		return nil, err
	}
	reg, err := os.ReadFile(node + "reg")
	if err != nil {
		return nil, err
	}
	found := false
	for _, s := range bytes.Split(compatible, []byte{0}) {
		if string(s) == "rockchip,rk3506-ioc-pmu" {
			found = true
		}
	}
	if !found || len(reg) != 8 || binary.BigEndian.Uint32(reg) != 0xff950000 || binary.BigEndian.Uint32(reg[4:]) != 0x10000 {
		return nil, errors.New("不支持的 IOC_PMU 设备树")
	}
	fd, err := unix.Open("/dev/mem", unix.O_RDWR|unix.O_SYNC|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	mapping, err := unix.Mmap(fd, 0xff950000, 4096, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	unix.Close(fd)
	if err != nil {
		return nil, err
	}
	register := (*uint32)(unsafe.Pointer(&mapping[0x200]))
	const mask = uint32(0x0ff0)
	previous := atomic.LoadUint32(register) & mask
	restore := func() { atomic.StoreUint32(register, mask<<16|previous); _ = unix.Munmap(mapping) }
	atomic.StoreUint32(register, mask<<16|0x0550)
	if atomic.LoadUint32(register)&mask != 0x0550 {
		restore()
		return nil, errors.New("按键上拉读回失败")
	}
	return restore, nil
}

type i2cMessage struct {
	address, flags, length, pad uint16
	buffer                      *byte
}
type i2cTransfer struct {
	messages *i2cMessage
	count    uint32
}
type adc struct{ fd uintptr }

func (d adc) transfer(payload, read []byte) error {
	messages := []i2cMessage{{address: 0x48, length: uint16(len(payload)), buffer: &payload[0]}}
	if len(read) > 0 {
		messages = append(messages, i2cMessage{address: 0x48, flags: 1, length: uint16(len(read)), buffer: &read[0]})
	}
	request := i2cTransfer{messages: &messages[0], count: uint32(len(messages))}
	result, _, errno := unix.Syscall(unix.SYS_IOCTL, d.fd, 0x0707, uintptr(unsafe.Pointer(&request)))
	runtime.KeepAlive(messages)
	runtime.KeepAlive(payload)
	runtime.KeepAlive(read)
	if errno != 0 {
		return errno
	}
	if int(result) != len(messages) {
		return errors.New("ADS1015 传输不完整")
	}
	return nil
}
func (d adc) read(reg byte) (uint16, error) {
	var b [2]byte
	err := d.transfer([]byte{reg}, b[:])
	return binary.BigEndian.Uint16(b[:]), err
}
func (d adc) write(reg byte, value uint16) error {
	return d.transfer([]byte{reg, byte(value >> 8), byte(value)}, nil)
}
func (d adc) axis(ctx context.Context, channel int) (float64, error) {
	if err := d.write(1, 0x8383|uint16(4+channel)<<12); err != nil {
		return 0, err
	}
	deadline := time.Now().Add(50 * time.Millisecond)
	for {
		if !pause(ctx, time.Millisecond) {
			return 0, ctx.Err()
		}
		config, err := d.read(1)
		if err != nil {
			return 0, err
		}
		if config&0x8000 != 0 {
			break
		}
		if time.Now().After(deadline) {
			return 0, errors.New("ADS1015 转换超时")
		}
	}
	raw, err := d.read(0)
	return float64(int16(raw) >> 4), err
}
func pause(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func joystick(ctx context.Context, options Options, emit func(Event)) (result error) {
	f, err := os.OpenFile("/dev/i2c-2", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	// Address-specific lock; the battery uses a different ADC on the same bus.
	lock, err := os.OpenFile("/tmp/hachimodock-ads1015.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("ADS1015 已占用: %w", err)
	}
	if err = ioctl(f.Fd(), 0x0703, 0x48); err != nil {
		return err
	}
	d := adc{fd: f.Fd()}
	previous, err := d.read(1)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, d.write(1, previous&0x7fff)) }()
	var samples [2][]float64
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		for channel := 0; channel < 2; channel++ {
			v, err := d.axis(ctx, channel)
			if err != nil {
				return err
			}
			samples[channel] = append(samples[channel], v)
		}
		if !pause(ctx, 10*time.Millisecond) {
			return nil
		}
	}
	var center [2]float64
	for i, s := range samples {
		sort.Float64s(s)
		center[i] = s[len(s)/2]
		if center[i] < 350 || center[i] > 1300 {
			return errors.New("摇杆中心超出范围，请松开摇杆后重启")
		}
	}
	log.Printf("摇杆校准完成: X=%.0f Y=%.0f", center[0], center[1])
	nav := navigator{options: options}
	for {
		x, err := d.axis(ctx, 0)
		if err != nil {
			return err
		}
		y, err := d.axis(ctx, 1)
		if err != nil {
			return err
		}
		for _, e := range nav.update(normalize(x, center[0]), normalize(y, center[1]), time.Now()) {
			emit(e)
		}
		if !pause(ctx, 20*time.Millisecond) {
			return nil
		}
	}
}
