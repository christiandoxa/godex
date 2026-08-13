package account

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (store *FileStore) CreateStagedHome() (string, error) {
	if err := store.Prepare(); err != nil {
		return "", fmt.Errorf("prepare account store: %w", err)
	}
	path, err := os.MkdirTemp(store.TempDir(), "login-")
	if err != nil {
		return "", fmt.Errorf("create staged Codex home: %w", err)
	}
	return path, nil
}

func (store *FileStore) RemoveStagedHome(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("staged Codex home is required")
	}
	if err := store.checkRoot(); err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve staged Codex home: %w", err)
	}
	temp, err := filepath.Abs(store.TempDir())
	if err != nil {
		return fmt.Errorf("resolve Godex temp directory: %w", err)
	}
	relative, err := filepath.Rel(temp, absolute)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return errors.New("staged Codex home must be a child of GODEX_HOME/tmp")
	}
	tempInfo, err := os.Lstat(temp)
	if err != nil {
		return fmt.Errorf("inspect Godex temp directory: %w", err)
	}
	if tempInfo.Mode()&os.ModeSymlink != 0 || !tempInfo.IsDir() {
		return errors.New("Godex temp directory must be a real directory")
	}
	resolvedTemp, err := filepath.EvalSymlinks(temp)
	if err != nil {
		return fmt.Errorf("resolve Godex temp directory: %w", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve staged Codex home parent: %w", err)
	}
	resolvedRelative, err := filepath.Rel(resolvedTemp, resolvedParent)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(os.PathSeparator)) {
		return errors.New("staged Codex home must resolve beneath GODEX_HOME/tmp")
	}
	return os.RemoveAll(absolute)
}

func (store *FileStore) validateStagedHome(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("staged Codex home is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve staged Codex home: %w", err)
	}
	temp, err := filepath.Abs(store.TempDir())
	if err != nil {
		return fmt.Errorf("resolve Godex temp directory: %w", err)
	}
	relative, err := filepath.Rel(temp, absolute)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || relative == ".." {
		return errors.New("staged Codex home must be a child of GODEX_HOME/tmp")
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return fmt.Errorf("inspect staged Codex home: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("staged Codex home is not a directory")
	}
	resolvedTemp, err := filepath.EvalSymlinks(temp)
	if err != nil {
		return fmt.Errorf("resolve Godex temp directory: %w", err)
	}
	resolvedHome, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return fmt.Errorf("resolve staged Codex home: %w", err)
	}
	resolvedRelative, err := filepath.Rel(resolvedTemp, resolvedHome)
	if err != nil || resolvedRelative == "." || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(os.PathSeparator)) {
		return errors.New("staged Codex home must resolve beneath GODEX_HOME/tmp")
	}
	return nil
}
