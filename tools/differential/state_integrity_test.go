package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureStateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "codex", "history.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDurableStateIntegrityRequiresCleanSyntheticWorkspace(t *testing.T) {
	root := fixtureStateRoot(t)
	if errors := auditFixtureDurableState(root); len(errors) != 0 {
		t.Fatalf("valid empty synthetic state rejected: %v", errors)
	}
}

func TestDurableStateIntegrityDetectsIndependentCorruption(t *testing.T) {
	fixtures := []struct {
		name   string
		path   string
		value  string
		prefix string
	}{
		{"history_pollution", "codex/history.jsonl", "{}\n", "unexpected_codex_history"},
		{"unauthorized_rollout", "codex/sessions/foreign.jsonl", "{}\n", "unexpected_persistent_session"},
		{"bad_routing_json", "state/routing.json", "{broken", "invalid_state_json"},
		{"secret_leak", "state/retry-backoff.json", fmt.Sprintf(`{"key":%q}`, apiKey), "synthetic_provider_secret_persisted"},
		{"bad_child_evidence", "child-exchange.json", "{broken", "invalid_client_evidence"},
		{"unexpected_file", "state/unknown-critical.bin", "x", "unknown_persistent_file"},
		{"unexpected_user_file", "user/.config/service-account.json", "{}", "unknown_persistent_file"},
		{"route_poisoning", "state/route-memory.json", `{"version":1,"scores":[{"account_id":"fake","score":99}]}`, "stale_provider_state:scores"},
		{"log_corruption", "state/logs/runtime.jsonl", "this is not json", "invalid_runtime_log_json"},

		{"quarantine_after_recovery", "state/retry-backoff.json", `{"version":1,"backoffs":[{"account_id":"synthetic","until_unix":9999999999}]}`, "stale_provider_state:backoffs"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			root := fixtureStateRoot(t)
			path := filepath.Join(root, fixture.path)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(fixture.value), 0o600); err != nil {
				t.Fatal(err)
			}
			errors := auditFixtureDurableState(root)
			found := false
			for _, message := range errors {
				if strings.HasPrefix(message, fixture.prefix) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("corrupted state escaped: %v", errors)
			}
		})
	}
}

func TestDurableStateIntegrityRejectsDivergentRecoverySnapshot(t *testing.T) {
	root := fixtureStateRoot(t)
	for name, payload := range map[string]string{
		"state/routing.json":           `{"owner":"A"}`,
		"state/routing.json.last-good": `{"owner":"B"}`,
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if failures := auditFixtureDurableState(root); len(failures) == 0 {
		t.Fatal("routing state and recovery backup diverged undetected")
	}
}

func TestHealthScoreSidecarTempAllowlistRejectsFilenameSpoofing(t *testing.T) {
	for _, name := range []string{
		"state/runtime-scores.json", "state/runtime-scores.json.lock",
		"state/state.json.lock", "state/runtime-scores.json.1234.5678.2.tmp",
	} {
		if !allowedFixtureStateFile(name) {
			t.Fatalf("known Rust sidecar rejected: %s", name)
		}
	}
	for _, name := range []string{
		"state/runtime-scores.json.evil.123.2.tmp",
		"state/runtime-scores.json.1.2.tmp",
		"state/runtime-scores.json.1.2.3.tmp.more",
		"state/runtime-scores.json.1.2.3.tmp/payload",
		"state/runtime-scores.json.1.2.3.bak",
	} {
		if allowedFixtureStateFile(name) {
			t.Fatalf("spoofed Rust sidecar accepted: %s", name)
		}
	}
}
