//go:build unix

package superexpose

import (
	"encoding/binary"
	"errors"
	"os"
	"syscall"
)

func openedFileIdentity(_ string, file *os.File, pathInfo os.FileInfo) ([]byte, error) {
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	pathStat, ok := pathInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("path file identity unavailable")
	}
	openedStat, ok := openedInfo.Sys().(*syscall.Stat_t)
	if !ok || pathStat.Dev != openedStat.Dev || pathStat.Ino != openedStat.Ino {
		return nil, errors.New("opened file identity changed")
	}
	bytes := make([]byte, 16)
	binary.LittleEndian.PutUint64(bytes[:8], uint64(pathStat.Dev))
	binary.LittleEndian.PutUint64(bytes[8:], uint64(pathStat.Ino))
	return bytes, nil
}
