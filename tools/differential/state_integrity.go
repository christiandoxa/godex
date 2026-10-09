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
		if info.Mode().Perm()&0o002 != 0 {
			violations = append(violations, "world_writable_file:"+relative)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(apiKey)) {
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
		case "state/logs/runtime.jsonl":
			for _, line := range bytes.Split(data, []byte("\n")) {
				if len(bytes.TrimSpace(line)) != 0 && !json.Valid(line) {
					violations = append(violations, "invalid_runtime_log_json")
					break
				}
			}
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
		"state/state.guard":
		return true
	}
	return strings.HasPrefix(name, "state/runtime-broker-live-runtime-") &&
		strings.HasSuffix(name, ".json.lock") && len(name) >= 48 && len(name) <= 160
}
