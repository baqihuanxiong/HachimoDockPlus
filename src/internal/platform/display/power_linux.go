//go:build linux

// Package display controls framebuffer output without suspending the system.
package display

import (
	"os"

	"golang.org/x/sys/unix"
)

type Power struct{ file *os.File }

func Open(framebuffer string) (*Power, error) {
	f, err := os.OpenFile(framebuffer, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	p := &Power{file: f}
	// Recover display output after an interrupted previous process as well.
	if err = p.SetOff(false); err != nil {
		f.Close()
		return nil, err
	}
	return p, nil
}

func (p *Power) SetOff(off bool) error {
	value := uintptr(0) // FB_BLANK_UNBLANK
	if off {
		value = 4 // FB_BLANK_POWERDOWN
	}
	// FBIOBLANK takes an integer, rather than a pointer to an integer.
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, p.file.Fd(), 0x4611, value)
	if errno != 0 {
		return errno
	}
	return nil
}

func (p *Power) Close() error {
	err := p.SetOff(false)
	closeErr := p.file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
