package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMergeCodexHistoryMatchesProdexDedupAndTimestampOrder(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "history.jsonl")
	source := filepath.Join(root, "profile-history.jsonl")
	old := `{"ts":2,"text":"b"}`
	first := `{"ts":1,"text":"a"}`
	plain := `plain`
	if err := os.WriteFile(destination, []byte(old+"\n"+first+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(old+"\n"+plain+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MergeCodexHistory(source, destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), first+"\n"+old+"\n"+plain; got != want {
		t.Fatalf("merged history = %q, want %q", got, want)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o640 {
			t.Fatalf("destination mode = %o, want 640", info.Mode().Perm())
		}
	}
}

func TestMergeCodexHistoryRejectsOversizedInputWithoutChangingDestination(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "history.jsonl")
	source := filepath.Join(root, "oversized.jsonl")
	original := []byte(`{"ts":1,"text":"keep"}`)
	if err := os.WriteFile(destination, original, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxCodexHistoryMergeBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := MergeCodexHistory(source, destination); err == nil || !strings.Contains(err.Error(), "exceeds safe size limit") {
		t.Fatalf("oversized merge error = %v", err)
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != string(original) {
		t.Fatalf("destination changed: %q err=%v", content, err)
	}
}
