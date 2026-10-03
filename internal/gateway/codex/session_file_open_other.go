//go:build !linux && !darwin && !windows

package codex

import (
	"errors"
	"os"
)

func openSessionFileNoFollow(string) (*os.File, error) {
	return nil, errors.New("no-follow session file opening is unsupported on this platform")
}

func sessionOpenedFileMatchesPath(os.FileInfo, string, *os.File) (bool, error) {
	return false, nil
}
