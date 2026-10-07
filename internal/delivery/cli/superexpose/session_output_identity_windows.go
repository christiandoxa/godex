//go:build windows

package superexpose

import (
	"encoding/binary"
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func openedFileIdentity(_ string, file *os.File, pathInfo os.FileInfo) ([]byte, error) {
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(pathInfo, openedInfo) {
		return nil, errors.New("opened file identity changed")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return nil, err
	}
	identity := make([]byte, 12)
	binary.LittleEndian.PutUint32(identity[:4], info.VolumeSerialNumber)
	binary.LittleEndian.PutUint32(identity[4:8], info.FileIndexHigh)
	binary.LittleEndian.PutUint32(identity[8:12], info.FileIndexLow)
	return identity, nil
}
