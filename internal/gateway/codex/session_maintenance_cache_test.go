package codex

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSessionMaintenanceCacheMatchesProdexSchema(t *testing.T) {
	if sessionMaintenanceCacheVersion != 4 || sessionMaintenanceCacheFile != "shared-codex-session-maintenance-v1.json" {
		t.Fatalf("cache constants = version:%d file:%q", sessionMaintenanceCacheVersion, sessionMaintenanceCacheFile)
	}
	fingerprint := sessionFileFingerprintValue{
		Len: 12, ModifiedSecs: 13, ModifiedNanos: 14, ChangedSecs: 15, ChangedNanos: 16, Identity: 17,
	}
	content, err := encodeSessionMaintenanceCache(sessionMaintenanceCache{
		Version: 4, Files: map[string]sessionFileFingerprintValue{"sessions/example.jsonl": fingerprint},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"version":4`, `"files":`, `"len":12`, `"modified_secs":13`, `"modified_nanos":14`, `"changed_secs":15`, `"changed_nanos":16`, `"identity":17`} {
		if !strings.Contains(string(content), field) {
			t.Fatalf("cache JSON %q missing %s", content, field)
		}
	}
}

func TestLoadSessionMaintenanceCacheRejectsMalformedAndOldVersions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, sessionMaintenanceCacheFile)
	for _, content := range []string{
		`not-json`,
		`{"version":4}`,
		`{"version":4,"files":null}`,
		`{"version":2,"files":{"sessions/a.jsonl":{"len":1,"modified_secs":2,"modified_nanos":3,"changed_secs":4,"changed_nanos":5,"identity":6}}}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		got := loadSessionMaintenanceCache(path)
		if got.Version != 0 || len(got.Files) != 0 {
			t.Fatalf("invalid cache loaded as %#v", got)
		}
	}
}

func TestSaveSessionMaintenanceCacheRoundTripsAndReplaces(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", sessionMaintenanceCacheFile)
	first := sessionMaintenanceCache{Version: 4, Files: map[string]sessionFileFingerprintValue{
		"sessions/a.jsonl": {Len: 1, ModifiedSecs: 2, ModifiedNanos: 3, ChangedSecs: 4, ChangedNanos: 5, Identity: 6},
	}}
	if err := saveSessionMaintenanceCache(path, first); err != nil {
		t.Fatal(err)
	}
	if got := loadSessionMaintenanceCache(path); !reflect.DeepEqual(got, first) {
		t.Fatalf("cache round-trip = %#v, want %#v", got, first)
	}
	second := sessionMaintenanceCache{Version: 4, Files: map[string]sessionFileFingerprintValue{}}
	if err := saveSessionMaintenanceCache(path, second); err != nil {
		t.Fatal(err)
	}
	if got := loadSessionMaintenanceCache(path); !reflect.DeepEqual(got, second) {
		t.Fatalf("replacement cache = %#v, want %#v", got, second)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != sessionMaintenanceCacheFile {
		t.Fatalf("cache directory entries = %#v", entries)
	}
}

func TestSessionFileFingerprintDetectsContentAndIdentityChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := sessionFileFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	// Ensure filesystems with coarse timestamp resolution still see either length,
	// change time, or identity move when the file is replaced.
	time.Sleep(time.Millisecond)
	replacement := path + ".new"
	if err := os.WriteFile(replacement, []byte("second-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	second, err := sessionFileFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("fingerprint did not change: %#v", first)
	}
}
