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
		if relative == "user" && entry.IsDir() {
			return filepath.SkipDir // not a persistent provider or Codex state root
		}
		if entry.IsDir() {
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
		case "state/retry-backoff.json":
			var snapshot struct {
				Backoffs []json.RawMessage `json:"backoffs"`
			}
			if json.Unmarshal(data, &snapshot) != nil {
				violations = append(violations, "unreadable_retry_backoffs")
			} else if len(snapshot.Backoffs) != 0 {
				violations = append(violations, "stale_provider_retry_backoff")
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
