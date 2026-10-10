package routing

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const routingSnapshotBackupSuffix = ".last-good"

// readRoutingSnapshot loads a validated routing sidecar and falls back to its
// last-good copy when the primary is missing or invalid. The repair is best
// effort: a usable backup must remain usable when the primary cannot be
// rewritten.
func readRoutingSnapshot[T any](path string, maxBytes int64, parse func([]byte) (T, error)) (T, error) {
	return readRoutingSnapshotWithPermissions(path, maxBytes, false, parse)
}

func readPrivateRoutingSnapshot[T any](path string, maxBytes int64, parse func([]byte) (T, error)) (T, error) {
	return readRoutingSnapshotWithPermissions(path, maxBytes, true, parse)
}

func readRoutingSnapshotWithPermissions[T any](path string, maxBytes int64, private bool, parse func([]byte) (T, error)) (T, error) {
	primary, _, primaryFound, primaryErr := readRoutingSnapshotFile(path, maxBytes, private, parse)
	if primaryErr == nil && primaryFound {
		return primary, nil
	}

	backupPath := path + routingSnapshotBackupSuffix
	backup, backupContent, backupFound, backupErr := readRoutingSnapshotFile(backupPath, maxBytes, private, parse)
	if backupErr != nil {
		if primaryErr != nil {
			var zero T
			return zero, primaryErr
		}
		var zero T
		return zero, backupErr
	}
	if backupFound {
		_, _ = fileutil.AtomicWrite(path, backupContent)
		return backup, nil
	}
	if primaryErr != nil {
		var zero T
		return zero, primaryErr
	}
	return backup, nil
}

func readRoutingSnapshotFile[T any](path string, maxBytes int64, private bool, parse func([]byte) (T, error)) (T, []byte, bool, error) {
	var zero T
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return zero, nil, false, nil
	}
	if err != nil {
		return zero, nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes ||
		(private && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return zero, nil, true, errors.New("routing snapshot must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return zero, nil, true, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return zero, nil, true, err
	}
	if int64(len(content)) > maxBytes {
		return zero, nil, true, errors.New("routing snapshot exceeds size limit")
	}
	value, err := parse(content)
	if err != nil {
		return zero, content, true, err
	}
	return value, content, true, nil
}

func writeRoutingSnapshot(path string, content []byte, maxBytes int64, parse func([]byte) error) error {
	if int64(len(content)) > maxBytes {
		return errors.New("routing snapshot exceeds size limit")
	}
	if err := parse(content); err != nil {
		return fmt.Errorf("validate routing snapshot: %w", err)
	}
	if _, err := fileutil.AtomicWrite(path, content); err != nil {
		return err
	}
	if _, err := fileutil.AtomicWrite(path+routingSnapshotBackupSuffix, content); err != nil {
		return fmt.Errorf("write routing snapshot backup: %w", err)
	}
	return nil
}
