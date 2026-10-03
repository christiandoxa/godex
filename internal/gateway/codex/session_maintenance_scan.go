package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func collectSessionMaintenanceCandidates(
	codexHome, sessionsDir string,
	previous sessionMaintenanceCache,
	next *sessionMaintenanceCache,
) ([]string, error) {
	info, err := os.Stat(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect session maintenance directory: %w", err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	directory, err := os.Open(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read session maintenance directory: %w", err)
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read session maintenance directory entries: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close session maintenance directory: %w", closeErr)
	}

	if next.Files == nil {
		next.Files = make(map[string]sessionFileFingerprintValue)
	}
	var candidates []string
	for _, entry := range entries {
		path := filepath.Join(sessionsDir, entry.Name())
		if entry.IsDir() {
			nested, err := collectSessionMaintenanceCandidates(codexHome, path, previous, next)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, nested...)
			continue
		}
		if !entry.Type().IsRegular() || !sessionMaintenanceRolloutName(entry.Name()) {
			continue
		}
		key := path
		if relative, err := filepath.Rel(codexHome, path); err == nil {
			key = relative
		}
		fingerprint, err := sessionFileFingerprint(path)
		if err != nil {
			return nil, err
		}
		if sessionMaintenanceCacheHit(previous, key, fingerprint) {
			next.Files[key] = fingerprint
			continue
		}
		candidates = append(candidates, path)
	}
	return candidates, nil
}

func sessionMaintenanceCacheHit(
	previous sessionMaintenanceCache,
	key string,
	fingerprint sessionFileFingerprintValue,
) bool {
	if previous.Version != sessionMaintenanceCacheVersion || previous.Files == nil {
		return false
	}
	cached, exists := previous.Files[key]
	return exists && cached == fingerprint
}

func sessionMaintenanceRolloutName(name string) bool {
	return strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl.zst")
}
