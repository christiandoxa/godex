package fileutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const codexManagedPackagesDirectory = "packages"

// CopyCodexHome copies one Codex home into an empty managed destination.
// The root packages directory is installer-owned and is intentionally omitted.
func CopyCodexHome(source, destination string) (err error) {
	sourcePath, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolve Codex copy source: %w", err)
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("inspect Codex copy source: %w", err)
	}
	if !sourceInfo.IsDir() {
		return errors.New("Codex copy source is not a directory")
	}

	destinationPath, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve Codex copy destination: %w", err)
	}
	if sameCodexPath(sourcePath, destinationPath) {
		return errors.New("Codex copy source and destination are the same path")
	}

	destinationExisted := true
	destinationInfo, statErr := os.Stat(destinationPath)
	switch {
	case errors.Is(statErr, os.ErrNotExist):
		destinationExisted = false
	case statErr != nil:
		return fmt.Errorf("inspect Codex copy destination: %w", statErr)
	case !destinationInfo.IsDir():
		return errors.New("Codex copy destination is not a directory")
	default:
		entries, readErr := os.ReadDir(destinationPath)
		if readErr != nil {
			return fmt.Errorf("read Codex copy destination: %w", readErr)
		}
		if len(entries) != 0 {
			return errors.New("Codex copy destination already exists and is not empty")
		}
	}

	if err := ensurePrivateCodexDirectory(destinationPath); err != nil {
		return err
	}
	if !destinationExisted {
		defer func() {
			if err != nil {
				_ = os.RemoveAll(destinationPath)
			}
		}()
	}

	sourceRoot, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return fmt.Errorf("resolve Codex copy source: %w", err)
	}
	return copyCodexDirectory(sourceRoot, sourcePath, destinationPath, true)
}

func copyCodexDirectory(sourceRoot, source, destination string, root bool) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read Codex directory %s: %w", source, err)
	}
	for _, entry := range entries {
		if root && entry.Name() == codexManagedPackagesDirectory {
			continue
		}
		sourcePath := filepath.Join(source, entry.Name())
		destinationPath := filepath.Join(destination, entry.Name())
		entryInfo, err := entry.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("inspect Codex entry %s: %w", sourcePath, err)
		}
		fileType := entryInfo.Mode()
		switch {
		case fileType.IsDir():
			if err := ensurePrivateCodexDirectory(destinationPath); err != nil {
				return err
			}
			if err := copyCodexDirectory(sourceRoot, sourcePath, destinationPath, false); err != nil {
				return err
			}
		case fileType.IsRegular():
			if err := copyCodexRegularFile(sourcePath, destinationPath); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return err
			}
		case fileType&os.ModeSymlink != 0:
			if err := copyCodexSymlinkedFile(sourceRoot, sourcePath, destinationPath); err != nil {
				return err
			}
		default:
			// Prodex ignores sockets, devices, and other special entries.
			continue
		}
	}
	return nil
}

func copyCodexSymlinkedFile(sourceRoot, source, destination string) error {
	target, err := filepath.EvalSymlinks(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		// A transient/broken link is treated as absent, matching the tagged copy.
		return nil
	}
	relative, err := filepath.Rel(sourceRoot, target)
	if err != nil || relative == ".." || filepath.IsAbs(relative) ||
		(len(relative) > 3 && relative[:3] == ".."+string(os.PathSeparator)) {
		return nil
	}
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("symlink target %s is not a regular file", source)
	}
	return copyCodexRegularFile(target, destination)
}

func copyCodexRegularFile(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Codex copy source %s is not a regular file", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	openedInfo, err := input.Stat()
	if err != nil {
		return err
	}
	currentInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !currentInfo.Mode().IsRegular() ||
		!os.SameFile(info, openedInfo) ||
		!os.SameFile(info, currentInfo) {
		return fmt.Errorf("Codex copy source changed while opening %s", source)
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".codex-copy-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	copyErr := error(nil)
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		copyErr = err
	} else if _, err := io.Copy(temporary, input); err != nil {
		copyErr = err
	} else if err := temporary.Sync(); err != nil {
		copyErr = err
	}
	closeErr := temporary.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return err
	}
	if err := os.Chtimes(temporaryPath, fileAccessTime(info), info.ModTime()); err != nil {
		return err
	}
	if err := Replace(temporaryPath, destination); err != nil {
		return err
	}
	if err := SyncDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	return nil
}

func ensurePrivateCodexDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Codex destination %s must be a real directory", path)
	}
	return os.Chmod(path, 0o700)
}

func sameCodexPath(left, right string) bool {
	leftResolved, leftErr := filepath.EvalSymlinks(left)
	rightResolved, rightErr := filepath.EvalSymlinks(right)
	if leftErr == nil && rightErr == nil {
		return filepath.Clean(leftResolved) == filepath.Clean(rightResolved)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
