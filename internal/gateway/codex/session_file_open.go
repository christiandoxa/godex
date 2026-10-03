package codex

import (
	"errors"
	"fmt"
	"os"
)

func inspectSessionRegularFile(path string) (os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect session file: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("refusing to read session file through symlink")
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("session path is not a file")
	}
	return before, nil
}

func openSessionRegularFileNoFollow(path string) (*os.File, os.FileInfo, error) {
	before, err := inspectSessionRegularFile(path)
	if err != nil {
		return nil, nil, err
	}
	file, err := openSessionRegularFileFromMetadata(path, before)
	return file, before, err
}

func openSessionRegularFileFromMetadata(path string, before os.FileInfo) (*os.File, error) {
	file, err := openSessionFileNoFollow(path)
	if err != nil {
		return nil, fmt.Errorf("open session file without following links: %w", err)
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened session file: %w", err)
	}
	if !opened.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("opened session path is not a file")
	}
	matches, err := sessionOpenedFileMatchesPath(before, path, file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("recheck session file identity: %w", err)
	}
	if !matches {
		_ = file.Close()
		return nil, errors.New("session file changed while opening")
	}
	return file, nil
}
