//go:build !unix && !windows

package superexpose

import (
	"encoding/binary"
	"errors"
	"os"
)

func openedFileIdentity(path string, file *os.File, pathInfo os.FileInfo) ([]byte, error) {
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(pathInfo, openedInfo) {
		return nil, errors.New("opened file identity changed")
	}
	bytes := make([]byte, 16)
	binary.LittleEndian.PutUint64(bytes[:8], uint64(pathInfo.Size()))
	binary.LittleEndian.PutUint64(bytes[8:], uint64(pathInfo.ModTime().UnixNano()))
	_ = path
	return bytes, nil
}
