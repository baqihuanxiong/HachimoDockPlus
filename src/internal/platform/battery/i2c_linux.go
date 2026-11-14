//go:build linux

package battery

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

type i2cMessage struct {
	address, flags, length, pad uint16
	buffer                      *byte
}
type i2cTransfer struct {
	messages *i2cMessage
	count    uint32
}
type device struct {
	fd      uintptr
	address uint16
}

func (d device) transfer(payload []byte, read []byte) error {
	messages := []i2cMessage{{address: d.address, length: uint16(len(payload)), buffer: &payload[0]}}
	if len(read) > 0 {
		messages = append(messages, i2cMessage{address: d.address, flags: 1, length: uint16(len(read)), buffer: &read[0]})
	}
	request := i2cTransfer{messages: &messages[0], count: uint32(len(messages))}
	result, _, errno := syscall.Syscall(syscall.SYS_IOCTL, d.fd, 0x0707, uintptr(unsafe.Pointer(&request)))
	runtime.KeepAlive(messages)
	runtime.KeepAlive(payload)
	runtime.KeepAlive(read)
	if errno != 0 {
		return errno
	}
	if int(result) != len(messages) {
		return errors.New("I2C 电池传输不完整")
	}
	return nil
}
func (d device) read(register byte) (uint16, error) {
	b := make([]byte, 2)
	err := d.transfer([]byte{register}, b)
	return binary.BigEndian.Uint16(b), err
}
func (d device) write(register byte, value uint16) error {
	return d.transfer([]byte{register, byte(value >> 8), byte(value)}, nil)
}
func (r *Reader) Read(ctx context.Context) (Sample, error) {
	if err := ctx.Err(); err != nil {
		return Sample{}, err
	}
	f, err := os.OpenFile(r.Path, os.O_RDWR, 0)
	if err != nil {
		return Sample{}, err
	}
	defer f.Close()
	// Cooperate with battery_probe.py; never force a kernel-owned I2C address.
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return Sample{}, err
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x0703, uintptr(r.Address)); errno != 0 {
		return Sample{}, errno
	}
	return measure(ctx, device{fd: f.Fd(), address: r.Address})
}
