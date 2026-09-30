package lockfile

import (
	"os"
	"syscall"
	"unsafe"
)

var lockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

func lockDescriptor(file *os.File, shared bool) error {
	var overlapped syscall.Overlapped
	flags := uintptr(3)
	if shared {
		flags = 1
	}
	result, _, err := lockFileEx.Call(file.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if result != 0 {
		return nil
	}
	if err == syscall.Errno(33) {
		return ErrBusy
	}
	return err
}
