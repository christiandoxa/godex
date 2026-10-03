package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSessionAttachmentFilePlainAndIndependentZstdGolden(t *testing.T) {
	const want = "{\"timestamp\":\"2026-10-03T12:49:50Z\",\"type\":\"event\"}\n"
	plain := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(plain, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{plain, filepath.Join("testdata", "session-small.jsonl.zst")} {
		got, ok, err := readSessionAttachmentFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !ok || got != want {
			t.Fatalf("read %s = found:%t content:%q", path, ok, got)
		}
	}
}

func TestReadSessionAttachmentFileEnforcesTaggedBounds(t *testing.T) {
	root := t.TempDir()
	onDisk := filepath.Join(root, "oversized.jsonl")
	file, err := os.Create(onDisk)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(sessionAttachmentRewriteMaxBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := readSessionAttachmentFile(onDisk); err != nil || ok || got != "" {
		t.Fatalf("oversized disk file = found:%t content:%q err:%v", ok, got, err)
	}

	if got, ok, err := readSessionAttachmentFile(filepath.Join("testdata", "session-oversized.jsonl.zst")); err != nil || ok || got != "" {
		t.Fatalf("oversized decoded file = found:%t bytes:%d err:%v", ok, len(got), err)
	}
}

func TestReadSessionAttachmentFileRejectsInvalidUTF8AndSymlink(t *testing.T) {
	root := t.TempDir()
	invalid := filepath.Join(root, "invalid.jsonl")
	if err := os.WriteFile(invalid, []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readSessionAttachmentFile(invalid); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("invalid UTF-8 error = %v", err)
	}

	link := filepath.Join(root, "linked.jsonl")
	if err := os.Symlink(invalid, link); err == nil {
		if _, _, err := readSessionAttachmentFile(link); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
			t.Fatalf("symlink read error = %v", err)
		}
	}
}

func TestCompressedSessionFileDetectionIsExact(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{"rollout.jsonl.zst", true},
		{"rollout.JSONL.ZST", false},
		{"rollout.zst", false},
		{"rollout.jsonl", false},
	} {
		if got := isCompressedSessionFile(test.name); got != test.want {
			t.Errorf("compressed detection %q = %t, want %t", test.name, got, test.want)
		}
	}
}
