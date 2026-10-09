package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxFixtureStateFileBytes = 8 << 20

// auditFixtureDurableState inspects effects produced by the isolated synthetic
// provider workflow. It checks independent safety/integrity invariants, not
// private Rust-versus-Go runtime bookkeeping layout equality.
func auditFixtureDurableState(root string) []string {
	var violations []string
	var routing, lastGood []byte
	historyFound := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			// Node/npm compilation caches are volatile runner artifacts, not
			// durable application state or provider credentials. Inspect
			// every other HOME path to detect unintended persistence.
			if relative == "user/.npm" || relative == "user/node-compile-cache" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > maxFixtureStateFileBytes {
			violations = append(violations, "unexpected_file_type_or_size:"+relative)
			return nil
		}
		if !allowedFixtureStateFile(relative) {
			violations = append(violations, "unknown_persistent_file:"+relative)
		}
		// Go file mode bits do not describe Windows ACL permissions.
		// Apply POSIX permission assertions only where their semantics hold;
		// Windows still gets type, bounds, content, path, and secret checks.
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o002 != 0 {
			violations = append(violations, "world_writable_file:"+relative)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(apiKey)) || bytes.Contains(data, []byte(rotationPrimaryKey)) ||
			bytes.Contains(data, []byte(rotationSecondaryKey)) {
			violations = append(violations, "synthetic_provider_secret_persisted:"+relative)
		}
		switch relative {
		case "codex/history.jsonl":
			historyFound = true
			if len(data) != 0 {
				violations = append(violations, "unexpected_codex_history")
			}
		case "state/routing.json":
			routing = data
			if !emptyFixtureRoutingBindings(data) {
				violations = append(violations, "unexpected_ephemeral_affinity_binding")
			}
		case "state/routing.json.last-good":
			lastGood = data
		case "state/retry-backoff.json", "state/route-memory.json":
			var snapshot map[string]json.RawMessage
			if json.Unmarshal(data, &snapshot) != nil {
				violations = append(violations, "invalid_provider_state")
				break
			}
			var version int
			if json.Unmarshal(snapshot["version"], &version) != nil || version != 1 {
				violations = append(violations, "invalid_provider_state_version")
			}
			listKey := "backoffs"
			if relative == "state/route-memory.json" {
				listKey = "scores"
			}
			var entries []json.RawMessage
			if json.Unmarshal(snapshot[listKey], &entries) != nil || len(entries) != 0 {
				violations = append(violations, "stale_provider_state:"+listKey)
			}
		case "state/runtime-scores.json", "state/runtime-scores.json.last-good":
			if !validEmptyProdexHealthScoreSnapshot(data) {
				violations = append(violations, "nonempty_or_corrupt_prodex_health_scores")
			}
		case "state/logs/runtime.jsonl":
			for _, line := range bytes.Split(data, []byte("\n")) {
				if len(bytes.TrimSpace(line)) != 0 && !json.Valid(line) {
					violations = append(violations, "invalid_runtime_log_json")
					break
				}
			}
		}
		if strings.HasPrefix(relative, "state/runtime-scores.json.") &&
			strings.HasSuffix(relative, ".tmp") && allowedFixtureStateFile(relative) &&
			!validEmptyProdexHealthScoreSnapshot(data) {
			violations = append(violations, "nonempty_or_corrupt_prodex_health_scores")
		}
		if strings.HasPrefix(relative, "codex/sessions/") &&
			!strings.HasSuffix(relative, ".lock") {
			violations = append(violations, "unexpected_persistent_session:"+relative)
		}
		if strings.HasPrefix(relative, "state/") && strings.HasSuffix(relative, ".json") &&
			!json.Valid(data) {
			violations = append(violations, "invalid_state_json:"+relative)
		}
		if relative == "child-exchange.json" && !json.Valid(data) {
			violations = append(violations, "invalid_client_evidence")
		}
		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("state_walk_error:%T", err))
	}
	if !historyFound {
		violations = append(violations, "missing_codex_history")
	}
	if len(routing) != 0 || len(lastGood) != 0 {
		if len(routing) == 0 || len(lastGood) == 0 ||
			sha256.Sum256(routing) != sha256.Sum256(lastGood) {
			violations = append(violations, "routing_backup_diverges_from_durable_state")
		}
	}
	if errors.Is(err, os.ErrPermission) {
		violations = append(violations, "state_permission_denied")
	}
	return violations
}

// Only files observed in the exact tagged Prodex 0.436.1 and Godex fixture
// may appear. New profile, secret, or session files require explicit review.
func allowedFixtureStateFile(name string) bool {
	switch name {
	case "child-exchange.json", "codex/history.jsonl",
		"codex/sessions/.prodex-maintenance.lock",
		"state/profile-lifecycle.json.lock", "state/runtime-housekeeping.last-run",
		"state/runtime-housekeeping.lock", "state/update-check.lock",
		"state/logs/runtime.guard", "state/logs/runtime.jsonl",
		"state/previous-response-failures.guard", "state/profile-import-lifecycle.guard",
		"state/profiles.guard", "state/retry-backoff.json",
		"state/route-memory.json", "state/routing-health.guard",
		"state/routing-memory.guard", "state/routing-retry-backoff.guard",
		"state/routing-transport-backoff.guard", "state/routing.guard",
		"state/routing.json", "state/routing.json.last-good",
		"state/state.guard", "state/state.json.lock",
		"state/runtime-scores.json", "state/runtime-scores.json.lock",
		"state/runtime-scores.json.last-good":
		return true
	}
	if strings.HasPrefix(name, "state/runtime-broker-live-runtime-") &&
		strings.HasSuffix(name, ".json.lock") && len(name) >= 48 && len(name) <= 160 {
		return true
	}
	// Prodex writes versioned health-score sidecars atomically. A failed
	// write may leave a numbered .tmp; it is not the authoritative snapshot.
	// Limit this exception to the exact Rust writer's three numeric segments.
	prefix := "state/runtime-scores.json."
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".tmp") {
		return false
	}
	sequence := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".tmp")
	parts := strings.Split(sequence, ".")
	if len(parts) == 4 && parts[0] == "last-good" {
		parts = parts[1:]
	}
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 20 {
			return false
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return false
			}
		}
	}
	return true
}

// This fixture contains no persistent profile identities. A health-score
// snapshot containing nonempty scores would change routing semantics and must
// fail, regardless of whether it is a committed sidecar or a leftover temp.
func validEmptyProdexHealthScoreSnapshot(data []byte) bool {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) != nil || len(envelope) != 2 {
		return false
	}
	var generation uint64
	if json.Unmarshal(envelope["generation"], &generation) != nil || generation == 0 {
		return false
	}
	var scores map[string]json.RawMessage
	if json.Unmarshal(envelope["value"], &scores) != nil || scores == nil {
		return false
	}
	return len(scores) == 0
}

// The isolated API-key test has no managed profile sessions to persist.
// A serialized affinity would change credential selection after restart,
// as seen in the previous key-rotation bug.
func emptyFixtureRoutingBindings(data []byte) bool {
	var payload map[string]json.RawMessage
	if json.Unmarshal(data, &payload) != nil || len(payload) != 2 {
		return false
	}
	var version int
	if json.Unmarshal(payload["version"], &version) != nil || version != 1 {
		return false
	}
	var bindings []json.RawMessage
	if json.Unmarshal(payload["bindings"], &bindings) != nil || string(payload["bindings"]) == "null" {
		return false
	}
	return len(bindings) == 0
}
