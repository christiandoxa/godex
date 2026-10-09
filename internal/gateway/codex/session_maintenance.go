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
	if err := repairStateDBRolloutPaths(sharedCodexHome); err != nil {
		return fmt.Errorf("repair Codex state database rollout paths: %w", err)
	}

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
	stateDBCandidates, err := stateDBSessionCandidates(sharedCodexHome)
	if err != nil {
		return err
	}
	for _, candidate := range stateDBCandidates {
		if err := maintainManagedSessionFileWithSelector(sharedCodexHome, candidate.path, candidate.selector, &next); err != nil {
			return err
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

// MaintainSessions exposes the bounded pre-launch maintenance pass to the
// runtime use case without exposing Codex persistence details there.
func (process *CodexProcess) MaintainSessions(sharedCodexHome, cacheRoot string) error {
	return MaintainManagedSessions(sharedCodexHome, cacheRoot)
}

func maintainManagedSessionFile(codexHome, sessionFile string, next *sessionMaintenanceCache) error {
	return maintainManagedSessionFileWithSelector(codexHome, sessionFile, "", next)
}

func maintainManagedSessionFileWithSelector(
	codexHome, sessionFile, selector string,
	next *sessionMaintenanceCache,
) error {
	contents, found, err := persistSessionFileAttachments(codexHome, sessionFile)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	var repaired bool
	if selector == "" {
		repaired, err = repairSessionMetadataPrefix(sessionFile, contents)
	} else {
		repaired, err = repairSessionMetadataPrefixWithSelector(sessionFile, selector)
	}
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
