package profile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
)

func (store *Store) prepareProfileHome(value profileentity.Profile, source string, insecure bool) (bool, error) {
	if value.Managed && filepath.Clean(value.CodexHome) != filepath.Clean(store.ManagedHome(value.Name)) {
		return false, errors.New("managed profile home must be inside GODEX_HOME/profiles")
	}
	if strings.TrimSpace(source) != "" {
		return prepareCopiedProfileHome(value.CodexHome, source, insecure)
	}
	return prepareRegisteredProfileHome(value, insecure)
}

func prepareCopiedProfileHome(destination, source string, insecure bool) (bool, error) {
	if err := validateSourceDirectory(source, insecure); err != nil {
		return false, err
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return false, fmt.Errorf("profile home %s already exists", destination)
		}
		return false, err
	}
	if err := copyDirectory(source, destination); err != nil {
		_ = os.RemoveAll(destination)
		return false, err
	}
	return true, nil
}

func prepareRegisteredProfileHome(value profileentity.Profile, insecure bool) (bool, error) {
	info, err := os.Lstat(value.CodexHome)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, errors.New("profile CODEX_HOME must be a real directory")
		}
		if !value.Managed && !insecure {
			return false, requirePrivateDirectory(value.CodexHome, info)
		}
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(value.CodexHome, 0o700); err != nil {
		return false, err
	}
	return true, nil
}

func validateSourceDirectory(path string, insecure bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect source CODEX_HOME: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("source CODEX_HOME must be a real directory")
	}
	if insecure {
		return nil
	}
	return requirePrivateDirectory(path, info)
}

func requirePrivateDirectory(path string, info fs.FileInfo) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("CODEX_HOME %s is accessible by group or others; pass --insecure to bypass this check", path)
	}
	return nil
}

func copyDirectory(source, destination string) error {
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source CODEX_HOME contains symbolic link %s", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Mkdir(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source CODEX_HOME contains unsupported file %s", path)
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	permission := os.FileMode(0o600)
	if mode.Perm()&0o111 != 0 {
		permission = 0o700
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, permission)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	return errors.Join(copyErr, closeErr)
}
