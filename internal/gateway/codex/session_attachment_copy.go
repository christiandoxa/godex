package codex

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func sessionPathIsRegularFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect attachment path: %w", err)
	}
	return info.Mode().IsRegular(), nil
}

func copySessionAttachmentFile(source, destination string) error {
	info, err := inspectSessionRegularFile(source)
	if err != nil {
		return err
	}
	input, err := openSessionRegularFileFromMetadata(source, info)
	if err != nil {
		return err
	}
	defer input.Close()

	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create attachment destination directory: %w", err)
	}
	if destinationInfo, statErr := os.Lstat(destination); statErr == nil {
		if destinationInfo.IsDir() && destinationInfo.Mode()&os.ModeSymlink == 0 {
			return errors.New("attachment destination is a directory")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect attachment destination: %w", statErr)
	}

	temp, err := os.CreateTemp(parent, ".godex-attachment-*.tmp")
	if err != nil {
		return fmt.Errorf("create attachment temporary file: %w", err)
	}
	tempPath := temp.Name()
	keepTemp := true
	defer func() {
		_ = temp.Close()
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := io.Copy(temp, input); err != nil {
		return fmt.Errorf("copy attachment data: %w", err)
	}
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("preserve attachment permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync attachment temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close attachment temporary file: %w", err)
	}
	if err := os.Chtimes(tempPath, sessionFileAccessTime(info), info.ModTime()); err != nil {
		return fmt.Errorf("preserve attachment timestamps: %w", err)
	}
	if err := replaceSessionAttachmentFile(tempPath, destination); err != nil {
		return err
	}
	keepTemp = false
	if runtime.GOOS != "windows" {
		directory, err := os.Open(parent)
		if err != nil {
			return fmt.Errorf("open attachment destination directory: %w", err)
		}
		syncErr := directory.Sync()
		closeErr := directory.Close()
		if syncErr != nil {
			return fmt.Errorf("sync attachment destination directory: %w", syncErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close attachment destination directory: %w", closeErr)
		}
	}
	return nil
}

func replaceSessionAttachmentFile(tempPath, destination string) error {
	if err := os.Rename(tempPath, destination); err == nil {
		return nil
	} else if info, statErr := os.Lstat(destination); statErr == nil {
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return errors.New("attachment destination is a directory")
		}
		if removeErr := os.Remove(destination); removeErr != nil {
			return fmt.Errorf("remove attachment destination: %w", removeErr)
		}
		if secondErr := os.Rename(tempPath, destination); secondErr != nil {
			return fmt.Errorf("replace attachment destination: %w", secondErr)
		}
		return nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect attachment destination after rename failure: %w", statErr)
	} else {
		return fmt.Errorf("replace attachment destination: %w", err)
	}
}

func sessionClipboardSourcePersistable(source string) bool {
	root := filepath.Clean(os.TempDir())
	path := filepath.Clean(source)
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
