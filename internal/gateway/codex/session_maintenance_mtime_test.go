package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLastSessionEventTimestampUsesLastValidRFC3339Value(t *testing.T) {
	contents := "" +
		`{"timestamp":"2026-10-03T10:11:12Z","type":"first"}` + "\n" +
		`{"timestamp":"not-a-time","type":"ignored"}` + "\n" +
		`{"timestamp":"2026-10-03T19:49:50.123456789+07:00","type":"last"}` + "\n" +
		`{"type":"no-timestamp"}` + "\n"
	got, ok := lastSessionEventTimestamp(contents)
	if !ok {
		t.Fatal("last session timestamp was not found")
	}
	want := time.Date(2026, 10, 3, 12, 49, 50, 123456789, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("last timestamp = %s, want %s", got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

func TestLastSessionEventTimestampMatchesProdexPrefixScanner(t *testing.T) {
	contents := `prefix {"timestamp":"2026-10-03T12:00:00Z"} suffix` + "\n" +
		`{"other":"timestamp","timestamp" : "2026-10-03T13:00:00Z"}` + "\n"
	got, ok := lastSessionEventTimestamp(contents)
	if !ok || got.Format(time.RFC3339) != "2026-10-03T12:00:00Z" {
		t.Fatalf("prefix-scanned timestamp = %s, found=%t", got.Format(time.RFC3339Nano), ok)
	}
}

func TestRestoreSessionFileModifiedTimePreservesAccessTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	access := time.Unix(1_700_000_123, 456_000_000)
	modified := time.Unix(1_700_000_456, 789_000_000)
	if err := os.Chtimes(path, access, modified); err != nil {
		t.Fatal(err)
	}
	contents := `{"timestamp":"2026-10-03T12:49:50.123456789Z"}` + "\n"
	if err := restoreSessionFileModifiedTime(path, contents); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	wantModified := time.Date(2026, 10, 3, 12, 49, 50, 123456789, time.UTC)
	if got := info.ModTime(); !timestampsWithin(got, wantModified, time.Microsecond) {
		t.Fatalf("mtime = %s, want %s", got.Format(time.RFC3339Nano), wantModified.Format(time.RFC3339Nano))
	}
	if got := sessionFileAccessTime(info); !timestampsWithin(got, access, time.Microsecond) {
		t.Fatalf("atime = %s, want %s", got.Format(time.RFC3339Nano), access.Format(time.RFC3339Nano))
	}
}

func TestRestoreSessionFileModifiedTimeNoTimestampIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	access := time.Unix(1_700_000_123, 0)
	modified := time.Unix(1_700_000_456, 0)
	if err := os.Chtimes(path, access, modified); err != nil {
		t.Fatal(err)
	}
	if err := restoreSessionFileModifiedTime(path, `{"type":"event"}`); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !timestampsWithin(info.ModTime(), modified, time.Microsecond) ||
		!timestampsWithin(sessionFileAccessTime(info), access, time.Microsecond) {
		t.Fatalf("timestamps changed on no-op: atime=%s mtime=%s", sessionFileAccessTime(info), info.ModTime())
	}
}

func timestampsWithin(left, right time.Time, tolerance time.Duration) bool {
	delta := left.Sub(right)
	return delta >= -tolerance && delta <= tolerance
}
