//go:build linux

package clock

import (
	"errors"
	"syscall"
	"time"
)

func Set(t time.Time) error {
	tv := syscall.NsecToTimeval(t.UnixNano())
	if err := syscall.Settimeofday(&tv); err != nil {
		return errors.New("系统对时需要 root 或 CAP_SYS_TIME 权限")
	}
	return nil
}
