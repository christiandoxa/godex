package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSessionRegularFileNoFollowAcceptsStableRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, before, err := openSessionRegularFileNoFollow(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if before.Size() != 4 {
		t.Fatalf("metadata size = %d", before.Size())
	}
	buffer := make([]byte, 4)
	if _, err := file.Read(buffer); err != nil || string(buffer) != "safe" {
		t.Fatalf("read = %q, err=%v", buffer, err)
	}
}

func TestOpenSessionRegularFileNoFollowRejectsSymlinkAndDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.jsonl")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.jsonl")
	if err := os.Symlink(target, link); err == nil {
		if _, _, err := openSessionRegularFileNoFollow(link); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
			t.Fatalf("symlink error = %v", err)
		}
	}
	if _, _, err := openSessionRegularFileNoFollow(root); err == nil || !strings.Contains(err.Error(), "not a file") {
		t.Fatalf("directory error = %v", err)
	}
}

func TestSessionOpenedFileMatchesPathRejectsStalePreopenMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout.jsonl")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("second-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := openSessionFileNoFollow(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	matches, err := sessionOpenedFileMatchesPath(before, path, file)
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("replacement file matched stale pre-open metadata")
	}
}
