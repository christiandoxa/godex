package codex

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSessionMaintenanceCandidatesMatchProdexRolloutPolicy(t *testing.T) {
	root := t.TempDir()
	sessions := filepath.Join(root, "sessions")
	nested := filepath.Join(sessions, "2026", "10", "03")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"rollout-a.jsonl":      "a",
		"anything.jsonl":       "b",
		"compressed.jsonl.zst": "c",
		"legacy.json":          "d",
		"notes.txt":            "e",
	} {
		if err := os.WriteFile(filepath.Join(nested, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(nested, "linked.jsonl")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	next := sessionMaintenanceCache{Version: sessionMaintenanceCacheVersion, Files: map[string]sessionFileFingerprintValue{}}
	candidates, err := collectSessionMaintenanceCandidates(root, sessions, sessionMaintenanceCache{}, &next)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		got[filepath.Base(candidate)] = true
	}
	want := map[string]bool{"rollout-a.jsonl": true, "anything.jsonl": true, "compressed.jsonl.zst": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
	if len(next.Files) != 0 {
		t.Fatalf("uncached candidates unexpectedly entered next cache: %#v", next.Files)
	}
}

func TestSessionMaintenanceCandidatesReuseExactVersionedFingerprint(t *testing.T) {
	root := t.TempDir()
	sessions := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessions, "cached.jsonl")
	if err := os.WriteFile(path, []byte("cached"), 0o600); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := sessionFileFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	key, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	previous := sessionMaintenanceCache{
		Version: sessionMaintenanceCacheVersion,
		Files:   map[string]sessionFileFingerprintValue{key: fingerprint},
	}
	next := sessionMaintenanceCache{Version: sessionMaintenanceCacheVersion, Files: map[string]sessionFileFingerprintValue{}}
	candidates, err := collectSessionMaintenanceCandidates(root, sessions, previous, &next)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 || next.Files[key] != fingerprint {
		t.Fatalf("cache hit = candidates:%#v next:%#v", candidates, next)
	}

	previous.Version--
	next.Files = map[string]sessionFileFingerprintValue{}
	candidates, err = collectSessionMaintenanceCandidates(root, sessions, previous, &next)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || len(next.Files) != 0 {
		t.Fatalf("stale-version cache was reused: candidates:%#v next:%#v", candidates, next)
	}
}

func TestSessionMaintenanceCandidatesIgnoreMissingOrNonDirectoryRoot(t *testing.T) {
	root := t.TempDir()
	next := sessionMaintenanceCache{Version: sessionMaintenanceCacheVersion, Files: map[string]sessionFileFingerprintValue{}}
	for _, target := range []string{filepath.Join(root, "missing"), filepath.Join(root, "file")} {
		if filepath.Base(target) == "file" {
			if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		candidates, err := collectSessionMaintenanceCandidates(root, target, sessionMaintenanceCache{}, &next)
		if err != nil || len(candidates) != 0 {
			t.Fatalf("target %s = candidates:%#v err:%v", target, candidates, err)
		}
	}
}

func TestSessionMaintenanceCacheHitRequiresKeyPresence(t *testing.T) {
	zero := sessionFileFingerprintValue{}
	previous := sessionMaintenanceCache{
		Version: sessionMaintenanceCacheVersion,
		Files:   map[string]sessionFileFingerprintValue{},
	}
	if sessionMaintenanceCacheHit(previous, "sessions/missing.jsonl", zero) {
		t.Fatal("missing cache key matched a zero-valued fingerprint")
	}
	previous.Files["sessions/present.jsonl"] = zero
	if !sessionMaintenanceCacheHit(previous, "sessions/present.jsonl", zero) {
		t.Fatal("present cache key with exact fingerprint did not match")
	}
	previous.Version--
	if sessionMaintenanceCacheHit(previous, "sessions/present.jsonl", zero) {
		t.Fatal("stale cache version matched an exact fingerprint")
	}
}
