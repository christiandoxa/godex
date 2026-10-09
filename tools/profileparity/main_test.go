package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"testing"
)

func fixtureProfile(name, root string) map[string]any {
	return map[string]any{
		"name":       name,
		"codex_home": filepath.Join(root, "profiles", name),
		"managed":    true,
		"provider":   map[string]any{"provider_kind": "openai"},
	}
}
func fixtureProdexState(root string) map[string]any {
	return map[string]any{
		"schema_version": 1, "active_profile": "alpha",
		"profiles":                  map[string]any{"alpha": fixtureProfile("alpha", root)},
		"last_run_selected_at":      map[string]any{"alpha": 1791536276},
		"response_profile_bindings": map[string]any{},
		"session_profile_bindings":  map[string]any{},
	}
}
func fixtureGodexState(root string) map[string]any {
	return map[string]any{
		"version": 1, "active_profile": "alpha",
		"profiles": []any{fixtureProfile("alpha", root)},
	}
}
func serialize(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
func TestProdex04370IndependentManagedProfileStateProjection(t *testing.T) {
	root := t.TempDir()
	expected := stateProjection{Active: "alpha", Names: []string{"alpha"}}
	for _, product := range []string{"prodex", "godex"} {
		t.Run(product, func(t *testing.T) {
			var value any = fixtureGodexState(root)
			if product == "prodex" {
				value = fixtureProdexState(root)
			}
			got, err := decodeProfileState(product, serialize(t, value), root)
			if err != nil {
				t.Fatal(err)
			}
			if !equivalentStateProjection(got, expected) {
				t.Fatalf("%s profile projection incorrect: %+v", product, got)
			}
		})
	}
}
func TestProdex04370RejectsCorruptOrUnexpectedManagedState(t *testing.T) {
	root := t.TempDir()
	mutationProdex := []struct {
		name   string
		change func(map[string]any)
	}{
		{"schema", func(s map[string]any) { s["schema_version"] = 2 }},
		{"extra_state", func(s map[string]any) { s["private_credential"] = "should_fail" }},
		{"active_missing", func(s map[string]any) { s["active_profile"] = "missing" }},
		{"managed_false", func(s map[string]any) { s["profiles"].(map[string]any)["alpha"].(map[string]any)["managed"] = false }},
		{"foreign_provider", func(s map[string]any) {
			s["profiles"].(map[string]any)["alpha"].(map[string]any)["provider"] = map[string]any{"provider_kind": "gemini"}
		}},
		{"foreign_home", func(s map[string]any) {
			s["profiles"].(map[string]any)["alpha"].(map[string]any)["codex_home"] = filepath.Join(root, "..", "escape")
		}},
		{"nonempty_response_bindings", func(s map[string]any) { s["response_profile_bindings"] = map[string]any{"old": "account-a"} }},
		{"nonempty_session_bindings", func(s map[string]any) { s["session_profile_bindings"] = map[string]any{"old": "account-a"} }},
		{"stale_selection", func(s map[string]any) { s["last_run_selected_at"] = map[string]any{"deleted": 1791536276} }},
		{"invalid_selection_timestamp", func(s map[string]any) { s["last_run_selected_at"] = map[string]any{"alpha": -1} }},
	}
	for _, mut := range mutationProdex {
		t.Run("prodex/"+mut.name, func(t *testing.T) {
			value := fixtureProdexState(root)
			mut.change(value)
			if got, err := decodeProfileState("prodex", serialize(t, value), root); err == nil {
				t.Fatalf("corrupted state unexpectedly accepted: %+v", got)
			}
		})
	}
	godexMutations := []struct {
		name   string
		change func(map[string]any)
	}{
		{"extra_state", func(s map[string]any) { s["foreign"] = "unknown" }},
		{"duplicate_profile", func(s map[string]any) {
			s["profiles"] = []any{fixtureProfile("alpha", root), fixtureProfile("alpha", root)}
		}},
		{"missing_active", func(s map[string]any) { s["active_profile"] = "missing" }},
		{"invalid_managed", func(s map[string]any) {
			s["profiles"] = []any{map[string]any{"name": "alpha", "codex_home": filepath.Join(root, "profiles", "alpha"), "managed": false, "provider": map[string]any{"provider_kind": "openai"}}}
		}},
		{"invalid_version", func(s map[string]any) { s["version"] = 99 }},
	}
	for _, mut := range godexMutations {
		t.Run("godex/"+mut.name, func(t *testing.T) {
			value := fixtureGodexState(root)
			mut.change(value)
			if got, err := decodeProfileState("godex", serialize(t, value), root); err == nil {
				t.Fatalf("corrupted state unexpectedly accepted: %+v", got)
			}
		})
	}
}
func TestProdex04370ProfileLifecycleStageOracleIsIndependent(t *testing.T) {
	steps := profileSteps()
	if len(steps) != 20 {
		t.Fatalf("expected profile lifecycle coverage, got %d", len(steps))
	}
	if steps[0].name != "initial_list" || steps[len(steps)-1].name != "list_empty_after_external" {
		t.Fatalf("profile test stage coverage drifted: %+v", steps)
	}
	valid := stateProjection{Active: "beta", Names: []string{"beta"}}
	for _, bad := range []stateProjection{
		{Active: "alpha", Names: []string{"beta"}},
		{Active: "beta", Names: []string{"alpha", "beta"}},
		{Active: "beta", Names: []string{}},
	} {
		if equivalentStateProjection(valid, bad) {
			t.Fatalf("state oracle hid a semantic difference: %v", bad)
		}
	}
	if !equivalentStateProjection(valid, stateProjection{Active: "beta", Names: []string{"beta"}}) {
		t.Fatal("identical persisted state comparison failed")
	}
	if reflect.DeepEqual(profileSteps()[6].state, profileSteps()[8].state) != true {
		t.Fatal("failed profile use must preserve exact prior state")
	}
}
func TestProdex04370BinaryProvenanceRejectsDirtyAndStaleCandidates(t *testing.T) {
	const revision = "1615edbc5ed723d8e8d03705ba56071ac7d01c12"
	valid := []debug.BuildSetting{{Key: "vcs.revision", Value: revision}, {Key: "vcs.modified", Value: "false"}}
	if err := verifyBuildMetadata(valid, revision); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		settings []debug.BuildSetting
	}{
		{"dirty", []debug.BuildSetting{{Key: "vcs.revision", Value: revision}, {Key: "vcs.modified", Value: "true"}}},
		{"stale", []debug.BuildSetting{{Key: "vcs.revision", Value: "wrong"}, {Key: "vcs.modified", Value: "false"}}},
		{"absent", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := verifyBuildMetadata(test.settings, revision); err == nil {
				t.Fatal("invalid candidate executable accepted")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "impostor")
	if err := os.WriteFile(path, []byte("prodex 0.437.0"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := sha256File(path)
	if err != nil || sum == canonicalProdexBinarySHA256 {
		t.Fatal("binary content pin failed")
	}
}
func TestProdex04370ProfileStateWorldReadableIsRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file mode assertion")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "profiles"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "profiles.json")
	if err := os.WriteFile(path, serialize(t, fixtureGodexState(root)), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := profileFixture{name: "godex", configHome: root}
	if _, err := readAndValidateState(fixture); err == nil {
		t.Fatal("profile metadata accessible to group/other users accepted")
	}
}

func TestProdex04370ProfileRootSymlinkCannotEscapeIsolatedFixture(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "profiles")); err != nil {
		t.Skipf("symlink fixture not supported: %v", err)
	}
	fixture := profileFixture{name: "godex", configHome: root}
	if err := checkManagedHomes(fixture, false, false); err == nil {
		t.Fatal("symlinked managed-profile root escaped isolated state boundary")
	}
}

// External CODEX_HOME is user-owned. Correct metadata marks it unmanaged
// and removal must never mutate or delete the directory contents.
func TestProdex04370ExternalProfileMetadataAndHomeProtection(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "external")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "untouched.txt"), []byte("keep-external-home"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkExternalHome(profileFixture{externalHome: home}); err != nil {
		t.Fatal(err)
	}
	for _, product := range []string{"prodex", "godex"} {
		t.Run(product, func(t *testing.T) {
			value := fixtureProfile("external", filepath.Join(root, product))
			value["managed"] = false
			value["codex_home"] = home
			raw := serialize(t, value)
			if err := verifyEntry("external", raw, filepath.Join(root, product)); err != nil {
				t.Fatalf("valid externally owned profile metadata rejected: %v", err)
			}
			value["managed"] = true
			if err := verifyEntry("external", serialize(t, value), filepath.Join(root, product)); err == nil {
				t.Fatal("external profile was accepted as managed")
			}
			value["managed"] = false
			value["codex_home"] = filepath.Join(root, product, "profiles", "external")
			if err := verifyEntry("external", serialize(t, value), filepath.Join(root, product)); err == nil {
				t.Fatal("external profile path was falsely marked as protected user home")
			}
		})
	}
	if err := os.WriteFile(filepath.Join(home, "untouched.txt"), []byte("modified"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkExternalHome(profileFixture{externalHome: home}); err == nil {
		t.Fatal("foreign CODEX_HOME mutation escaped oracle")
	}
}
