package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	sessionMaintenanceCacheVersion = 4
	sessionMaintenanceCacheFile    = "shared-codex-session-maintenance-v1.json"
)

type sessionMaintenanceCache struct {
	Version uint8                                  `json:"version"`
	Files   map[string]sessionFileFingerprintValue `json:"files"`
}

type sessionFileFingerprintValue struct {
	Len           uint64 `json:"len"`
	ModifiedSecs  uint64 `json:"modified_secs"`
	ModifiedNanos uint32 `json:"modified_nanos"`
	ChangedSecs   int64  `json:"changed_secs"`
	ChangedNanos  int64  `json:"changed_nanos"`
	Identity      uint64 `json:"identity"`
}

func sessionFileFingerprint(path string) (sessionFileFingerprintValue, error) {
	info, err := os.Stat(path)
	if err != nil {
		return sessionFileFingerprintValue{}, fmt.Errorf("inspect session file: %w", err)
	}
	modifiedSecs, modifiedNanos := sessionModifiedParts(info.ModTime())
	changedSecs, changedNanos, identity := sessionFileChangedIdentity(info)
	return sessionFileFingerprintValue{
		Len: uint64(info.Size()), ModifiedSecs: modifiedSecs, ModifiedNanos: modifiedNanos,
		ChangedSecs: changedSecs, ChangedNanos: changedNanos, Identity: identity,
	}, nil
}

func sessionModifiedParts(modified time.Time) (uint64, uint32) {
	if modified.Before(time.Unix(0, 0)) {
		return 0, 0
	}
	return uint64(modified.Unix()), uint32(modified.Nanosecond())
}

func loadSessionMaintenanceCache(path string) sessionMaintenanceCache {
	content, err := os.ReadFile(path)
	if err != nil {
		return sessionMaintenanceCache{}
	}
	var cache sessionMaintenanceCache
	if json.Unmarshal(content, &cache) != nil || cache.Version != sessionMaintenanceCacheVersion || cache.Files == nil {
		return sessionMaintenanceCache{}
	}
	return cache
}

func encodeSessionMaintenanceCache(cache sessionMaintenanceCache) ([]byte, error) {
	return json.Marshal(cache)
}

func saveSessionMaintenanceCache(path string, cache sessionMaintenanceCache) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create session maintenance cache directory: %w", err)
	}
	content, err := encodeSessionMaintenanceCache(cache)
	if err != nil {
		return fmt.Errorf("serialize session maintenance cache: %w", err)
	}
	extension := filepath.Ext(path)
	stem := path[:len(path)-len(extension)]
	tempPath := stem + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	if err := os.WriteFile(tempPath, content, 0o600); err != nil {
		return fmt.Errorf("write session maintenance cache: %w", err)
	}
	if err := os.Rename(tempPath, path); err == nil {
		return nil
	} else if _, statErr := os.Stat(path); statErr == nil {
		if removeErr := os.Remove(path); removeErr != nil {
			_ = os.Remove(tempPath)
			return fmt.Errorf("remove session maintenance cache after rename failed: %w", removeErr)
		}
		if secondErr := os.Rename(tempPath, path); secondErr != nil {
			_ = os.Remove(tempPath)
			return fmt.Errorf("replace session maintenance cache: %w", secondErr)
		}
		return nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		_ = os.Remove(tempPath)
		return fmt.Errorf("inspect session maintenance cache after rename failed: %w", statErr)
	} else {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace session maintenance cache: %w", err)
	}
}
