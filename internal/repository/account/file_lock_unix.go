//go:build !windows

package account

import (
	"errors"
	"os"
	"syscall"
)

func lockDescriptor(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return errFileBusy
	}
	return err
}
