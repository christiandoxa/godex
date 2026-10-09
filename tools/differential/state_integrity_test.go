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

func TestEmptyProdexHealthScoreEnvelopeRejectsPersistedSelectionState(t *testing.T) {
	for _, input := range []string{
		`{"generation":1,"value":{}}`,
		`{"generation":20,"value":{}}`,
	} {
		if !validEmptyProdexHealthScoreSnapshot([]byte(input)) {
			t.Fatalf("valid empty score sidecar rejected: %s", input)
		}
	}
	for _, input := range []string{
		`{"generation":0,"value":{}}`,
		`{"generation":1,"value":{"profile-A":{"health":3}}}`,
		`{"generation":1,"value":null}`,
		`{"generation":1,"value":[]}`,
		`{"generation":1,"value":{},"unknown":"extra"}`,
		`{"generation":1,"value":`,
	} {
		if validEmptyProdexHealthScoreSnapshot([]byte(input)) {
			t.Fatalf("nonempty/corrupt Prodex score sidecar accepted: %s", input)
		}
	}
}
func TestProdexScoreTempWithMaterialHealthFailsAudit(t *testing.T) {
	root := fixtureStateRoot(t)
	path := filepath.Join(root, "state", "runtime-scores.json.12345.67890.2.tmp")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"generation":1,"value":{"profile-A":{"penalty":4}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, failure := range auditFixtureDurableState(root) {
		if strings.HasPrefix(failure, "nonempty_or_corrupt_prodex_health_scores") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("material Prodex health sidecar was not rejected")
	}
}

func TestSyntheticEphemeralAffinityStateMustBeEmpty(t *testing.T) {
	valid := []byte(`{"version":1,"bindings":[]}`)
	if !emptyFixtureRoutingBindings(valid) {
		t.Fatal("clean volatile provider routing state was rejected")
	}
	for _, bad := range [][]byte{
		[]byte(`{"version":1,"bindings":[{"kind":"previous","key":"abc","account_id":"def"}]}`),
		[]byte(`{"version":1,"bindings":null}`),
		[]byte(`{"version":2,"bindings":[]}`),
		[]byte(`{"version":1,"bindings":[],"hidden":1}`),
		[]byte(`{"version":1,"bindings":{}}`),
		[]byte(`{broken}`),
	} {
		if emptyFixtureRoutingBindings(bad) {
			t.Fatalf("persisted provider or corrupt routing state accepted: %s", bad)
		}
	}
}
func TestSyntheticEphemeralBindingCannotEscapeThroughStateAudit(t *testing.T) {
	root := fixtureStateRoot(t)
	path := filepath.Join(root, "state", "routing.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := []byte(`{"version":1,"bindings":[{"kind":"previous","key":"abc","account_id":"def"}]}`)
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, violation := range auditFixtureDurableState(root) {
		if strings.HasPrefix(violation, "unexpected_ephemeral_affinity_binding") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("durable previous_response binding escaped fixture oracle")
	}
}
