//go:build windows

package codex

import (
	"os"
	"syscall"
)

const sessionFileFlagOpenReparsePoint = 0x00200000

func openSessionFileNoFollow(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(
		name,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL|sessionFileFlagOpenReparsePoint,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

func sessionOpenedFileMatchesPath(before os.FileInfo, path string, file *os.File) (bool, error) {
	opened, err := file.Stat()
	if err != nil {
		return false, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !current.Mode().IsRegular() {
		return false, nil
	}
	beforeData, beforeOK := before.Sys().(*syscall.Win32FileAttributeData)
	openedData, openedOK := opened.Sys().(*syscall.Win32FileAttributeData)
	if !beforeOK || !openedOK {
		return false, nil
	}
	if beforeData.FileAttributes != openedData.FileAttributes ||
		beforeData.CreationTime != openedData.CreationTime ||
		beforeData.LastWriteTime != openedData.LastWriteTime ||
		beforeData.FileSizeHigh != openedData.FileSizeHigh ||
		beforeData.FileSizeLow != openedData.FileSizeLow {
		return false, nil
	}
	return os.SameFile(current, opened), nil
}
