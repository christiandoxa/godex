package codex

import (
	"fmt"
	"path/filepath"
	"reflect"
)

// MaintainManagedSessions runs the full Prodex-compatible shared-session maintenance pass.
func MaintainManagedSessions(sharedCodexHome, cacheRoot string) error {
	if err := validateCodexHomePath(sharedCodexHome); err != nil {
		return err
	}
	release, acquired, err := (SessionLocker{}).TryLockCodexSessionsForMaintenance(sharedCodexHome)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer release()

	cachePath := filepath.Join(cacheRoot, sessionMaintenanceCacheFile)
	previous := loadSessionMaintenanceCache(cachePath)
	next := sessionMaintenanceCache{
		Version: sessionMaintenanceCacheVersion,
		Files:   make(map[string]sessionFileFingerprintValue),
	}

	for _, directory := range []string{"sessions", "archived_sessions"} {
		candidates, err := collectSessionMaintenanceCandidates(
			sharedCodexHome, filepath.Join(sharedCodexHome, directory), previous, &next,
		)
		if err != nil {
			return err
		}
		for _, sessionFile := range candidates {
			if err := maintainManagedSessionFile(sharedCodexHome, sessionFile, &next); err != nil {
				return err
			}
		}
	}
	if err := persistSessionGoalAttachmentPaths(sharedCodexHome); err != nil {
		return err
	}
	if !reflect.DeepEqual(next, previous) {
		_ = saveSessionMaintenanceCache(cachePath, next)
	}
	return nil
}

func maintainManagedSessionFile(codexHome, sessionFile string, next *sessionMaintenanceCache) error {
	contents, found, err := persistSessionFileAttachments(codexHome, sessionFile)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	repaired, err := repairSessionMetadataPrefix(sessionFile, contents)
	if err != nil {
		return err
	}
	if repaired {
		contents, found, err = persistSessionFileAttachments(codexHome, sessionFile)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
	}
	if err := restoreSessionFileModifiedTime(sessionFile, contents); err != nil {
		return err
	}
	if !sessionAttachmentsAreStable(codexHome, contents) {
		return nil
	}
	key := sessionFile
	if relative, err := filepath.Rel(codexHome, sessionFile); err == nil {
		key = relative
	}
	fingerprint, err := sessionFileFingerprint(sessionFile)
	if err != nil {
		return fmt.Errorf("fingerprint maintained session %s: %w", sessionFile, err)
	}
	if next.Files == nil {
		next.Files = make(map[string]sessionFileFingerprintValue)
	}
	next.Files[key] = fingerprint
	return nil
}
