package account

import "syscall"

func processAlive(pid int) bool {
	handle, err := syscall.OpenProcess(0x1000, false, uint32(pid))
	if err != nil {
		return err != syscall.Errno(87)
	}
	defer syscall.CloseHandle(handle)
	var code uint32
	if syscall.GetExitCodeProcess(handle, &code) != nil {
		return true
	}
	return code == 259
}
