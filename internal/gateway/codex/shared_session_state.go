package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

var sharedSessionLinkSequence atomic.Uint64

var sharedCodexDirectories = [...]string{
	"sessions",
	"archived_sessions",
	"attachments",
	"image_attachments",
	"shell_snapshots",
	"memories",
	"memories_extensions",
	"rules",
	"skills",
	"agents",
	"plugins",
	filepath.Join(".tmp", "plugins"),
	filepath.Join(".tmp", "marketplaces"),
}

var sharedCodexStaticFiles = [...]string{
	"history.jsonl",
	"config.toml",
	"managed_config.toml",
	"environments.toml",
	"AGENTS.md",
	"AGENTS.override.md",
	"goals_1.sqlite",
	"goals_1.sqlite-shm",
	"goals_1.sqlite-wal",
	filepath.Join(".tmp", "plugins.sha"),
	filepath.Join(".tmp", "known_marketplaces.json"),
	filepath.Join(".tmp", "app-server-remote-plugin-sync-v1"),
}

// PrepareSharedSessionHome projects the session/index state needed by native
// Codex resume/fork onto one shared root while keeping profile credentials local.
// Configuration and other native Codex state follow the exact Prodex runtime manifest.
// This preserves the native Codex picker UI instead of
// reimplementing it in Godex.
func (process *CodexProcess) PrepareSharedSessionHome(profileHome, sharedHome string) error {
	if strings.TrimSpace(sharedHome) == "" {
		return nil
	}
	if err := validateCodexHomePath(profileHome); err != nil {
		return fmt.Errorf("invalid managed Codex home: %w", err)
	}
	if err := validateCodexHomePath(sharedHome); err != nil {
		return fmt.Errorf("invalid shared Codex home: %w", err)
	}
	if sameCodexHome(profileHome, sharedHome) {
		return nil
	}
	if err := ensureRealCodexDirectory(profileHome); err != nil {
		return fmt.Errorf("prepare managed Codex home: %w", err)
	}
	if err := ensureRealCodexDirectory(sharedHome); err != nil {
		return fmt.Errorf("prepare shared Codex home: %w", err)
	}

	for _, name := range sharedCodexDirectories {
		if err := migrateSharedCodexEntry(profileHome, sharedHome, name, true); err != nil {
			return err
		}
	}
	for _, name := range sharedCodexStaticFiles {
		if err := migrateSharedCodexEntry(profileHome, sharedHome, name, false); err != nil {
			return err
		}
	}
	dynamic, err := sharedCodexDynamicFiles(profileHome, sharedHome)
	if err != nil {
		return err
	}
	for _, name := range dynamic {
		if err := migrateSharedCodexEntry(profileHome, sharedHome, name, false); err != nil {
			return err
		}
	}
	return nil
}

func ensureRealCodexDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("Codex state root %s must be a real directory", path)
	}
	return os.Chmod(path, 0o700)
}

func migrateSharedCodexEntry(profileHome, sharedHome, name string, directory bool) error {
	local := filepath.Join(profileHome, name)
	shared := filepath.Join(sharedHome, name)
	metadata, err := os.Lstat(local)
	if errors.Is(err, os.ErrNotExist) {
		if directory {
			if err := ensureRealCodexDirectory(shared); err != nil {
				return fmt.Errorf("create shared Codex state %s: %w", name, err)
			}
		} else if err := os.MkdirAll(filepath.Dir(shared), 0o700); err != nil {
			return fmt.Errorf("create shared Codex state parent %s: %w", name, err)
		}
		return installSharedStateLink(local, shared, directory)
	}
	if err != nil {
		return fmt.Errorf("inspect managed Codex state %s: %w", name, err)
	}
	if metadata.Mode()&os.ModeSymlink != 0 {
		return migrateExistingSharedStateLink(local, shared, name, directory)
	}
	if directory {
		if !metadata.IsDir() {
			return fmt.Errorf("managed Codex state %s is not a directory", local)
		}
		if err := migrateSharedStateDirectory(local, shared); err != nil {
			return fmt.Errorf("migrate shared Codex directory %s: %w", name, err)
		}
	} else {
		if !metadata.Mode().IsRegular() {
			return fmt.Errorf("managed Codex state %s is not a regular file", local)
		}
		if err := migrateSharedStateFile(local, shared, name); err != nil {
			return fmt.Errorf("migrate shared Codex file %s: %w", name, err)
		}
	}
	return installSharedStateLink(local, shared, directory)
}

func migrateExistingSharedStateLink(local, shared, name string, directory bool) error {
	target, err := filepath.EvalSymlinks(local)
	if errors.Is(err, os.ErrNotExist) {
		if err := removeSharedStateLink(local); err != nil {
			return err
		}
		if directory {
			if err := ensureRealCodexDirectory(shared); err != nil {
				return err
			}
		}
		return installSharedStateLink(local, shared, directory)
	}
	if err != nil {
		return fmt.Errorf("resolve managed Codex state link %s: %w", local, err)
	}
	if sameCodexHome(target, shared) {
		return nil
	}
	if directory {
		if err := mergeSharedStateDirectoryTarget(target, shared); err != nil {
			return err
		}
	} else if _, err := os.Stat(target); err == nil {
		if err := migrateSharedStateFileFromLink(target, shared, name); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := removeSharedStateLink(local); err != nil {
		return err
	}
	return installSharedStateLink(local, shared, directory)
}

func mergeSharedStateDirectoryTarget(source, shared string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("legacy shared Codex state %s must be a real directory", source)
	}
	if _, err := os.Lstat(shared); errors.Is(err, os.ErrNotExist) {
		if err := ensureRealCodexDirectory(shared); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		sharedInfo, err := os.Lstat(shared)
		if err != nil {
			return err
		}
		if sharedInfo.Mode()&os.ModeSymlink != 0 || !sharedInfo.IsDir() {
			return fmt.Errorf("shared Codex state %s must be a real directory", shared)
		}
	}
	return fileutil.MergeCodexDirectory(source, shared)
}

func migrateSharedStateDirectory(local, shared string) error {
	if _, err := os.Lstat(shared); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(shared), 0o700); err != nil {
			return err
		}
		if err := os.Rename(local, shared); err == nil {
			return nil
		}
	} else if err != nil {
		return err
	}
	if err := fileutil.MergeCodexDirectory(local, shared); err != nil {
		return err
	}
	return os.RemoveAll(local)
}

func migrateSharedStateFile(local, shared, name string) error {
	if _, err := os.Lstat(shared); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(shared), 0o700); err != nil {
			return err
		}
		if err := os.Rename(local, shared); err == nil {
			return nil
		}
		if err := fileutil.CopyCodexFile(local, shared); err != nil {
			return err
		}
		return os.Remove(local)
	} else if err != nil {
		return err
	}
	if err := requireRegularSharedStateFile(shared); err != nil {
		return err
	}
	if filepath.Base(name) == "history.jsonl" {
		if err := fileutil.MergeCodexHistory(local, shared); err != nil {
			return err
		}
	}
	// Except history, Prodex treats an existing shared file as authoritative.
	return os.Remove(local)
}

func migrateSharedStateFileFromLink(target, shared, name string) error {
	if _, err := os.Lstat(shared); errors.Is(err, os.ErrNotExist) {
		return fileutil.CopyCodexFile(target, shared)
	} else if err != nil {
		return err
	}
	if err := requireRegularSharedStateFile(shared); err != nil {
		return err
	}
	if filepath.Base(name) == "history.jsonl" {
		return fileutil.MergeCodexHistory(target, shared)
	}
	return nil
}

func requireRegularSharedStateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("shared Codex state %s must be a real regular file", path)
	}
	return nil
}

func installSharedStateLink(local, shared string, directory bool) error {
	if metadata, err := os.Lstat(local); err == nil {
		if metadata.Mode()&os.ModeSymlink != 0 {
			target, resolveErr := filepath.EvalSymlinks(local)
			if resolveErr == nil && sameCodexHome(target, shared) {
				return nil
			}
		}
		return fmt.Errorf("managed Codex state path %s still exists before link install", local)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
		return err
	}
	var temporary string
	for attempt := 0; attempt < 64; attempt++ {
		sequence := sharedSessionLinkSequence.Add(1)
		candidate := local + fmt.Sprintf(".godex-link-%d-%d", os.Getpid(), sequence)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			temporary = candidate
			break
		} else if err != nil {
			return err
		}
	}
	if temporary == "" {
		return fmt.Errorf("reserve shared Codex state link beside %s", local)
	}
	if err := createSharedStateSymlink(shared, temporary, directory); err != nil {
		return fmt.Errorf("create shared Codex state link %s: %w", local, err)
	}
	if err := os.Rename(temporary, local); err != nil {
		_ = removeSharedStateLink(temporary)
		return fmt.Errorf("install shared Codex state link %s: %w", local, err)
	}
	return nil
}

func removeSharedStateLink(path string) error {
	metadata, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if metadata.IsDir() && metadata.Mode()&os.ModeSymlink == 0 {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func sameCodexHome(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	leftResolved, leftErr := filepath.EvalSymlinks(leftAbsolute)
	rightResolved, rightErr := filepath.EvalSymlinks(rightAbsolute)
	if leftErr == nil {
		leftAbsolute = leftResolved
	}
	if rightErr == nil {
		rightAbsolute = rightResolved
	}
	return filepath.Clean(leftAbsolute) == filepath.Clean(rightAbsolute)
}

func sharedCodexDynamicFiles(roots ...string) ([]string, error) {
	seen := map[string]struct{}{}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("read Codex state root %s: %w", root, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if sharedCodexSQLiteName(name) || sharedCodexProfileConfigName(name) {
				seen[name] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func sharedCodexSQLiteName(name string) bool {
	validPrefix := false
	for _, prefix := range []string{"state_", "logs_", "goals_", "memories_"} {
		if strings.HasPrefix(name, prefix) {
			validPrefix = true
			break
		}
	}
	if !validPrefix {
		return false
	}
	return strings.HasSuffix(name, ".sqlite") || strings.HasSuffix(name, ".sqlite-shm") || strings.HasSuffix(name, ".sqlite-wal")
}

func sharedCodexProfileConfigName(name string) bool {
	profile, ok := strings.CutSuffix(name, ".config.toml")
	if !ok || profile == "" {
		return false
	}
	for _, r := range profile {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}
