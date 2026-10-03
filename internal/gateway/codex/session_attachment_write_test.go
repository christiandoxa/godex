package codex

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func TestPersistSessionFileAttachmentsRewritesPlainRolloutAtomically(t *testing.T) {
	home := t.TempDir()
	sessionDir := filepath.Join(home, "sessions", "2026", "10", "03")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "codex-clipboard-write.png")
	if err := os.WriteFile(source, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(sessionDir, "rollout.jsonl")
	content := "<image path=\"" + source + "\">\n"
	if err := os.WriteFile(session, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	rewritten, ok, err := persistSessionFileAttachments(home, session)
	if err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(home, "image_attachments", filepath.Base(source))
	if !ok || !strings.Contains(rewritten, stable) || strings.Contains(rewritten, source) {
		t.Fatalf("persisted = ok:%t content:%q", ok, rewritten)
	}
	if got, err := os.ReadFile(session); err != nil || string(got) != rewritten {
		t.Fatalf("rollout = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(stable); err != nil || string(got) != "image" {
		t.Fatalf("stable image = %q, err=%v", got, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(session); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("rollout mode = %#v, err=%v", info, err)
		}
	}
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(session) {
		t.Fatalf("session directory leaked temporary files: %#v", entries)
	}

	unchangedTime := time.Unix(1_700_000_456, 0)
	if err := os.Chtimes(session, unchangedTime, unchangedTime); err != nil {
		t.Fatal(err)
	}
	if second, ok, err := persistSessionFileAttachments(home, session); err != nil || !ok || second != rewritten {
		t.Fatalf("second persist = ok:%t content:%q err:%v", ok, second, err)
	}
	if info, err := os.Stat(session); err != nil || !info.ModTime().Equal(unchangedTime) {
		t.Fatalf("no-op persist changed mtime: %#v err=%v", info, err)
	}
}

func TestPersistSessionFileAttachmentsRewritesZstdRollout(t *testing.T) {
	home := t.TempDir()
	sessionDir := filepath.Join(home, "archived_sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "codex-clipboard-zstd.png")
	if err := os.WriteFile(source, []byte("compressed-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(sessionDir, "rollout.jsonl.zst")
	content := "<image path=\"" + source + "\">\n"
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)))
	if err != nil {
		t.Fatal(err)
	}
	compressed := encoder.EncodeAll([]byte(content), nil)
	encoder.Close()
	if err := os.WriteFile(session, compressed, 0o600); err != nil {
		t.Fatal(err)
	}

	rewritten, ok, err := persistSessionFileAttachments(home, session)
	if err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(home, "image_attachments", filepath.Base(source))
	if !ok || !strings.Contains(rewritten, stable) {
		t.Fatalf("persisted compressed = ok:%t content:%q", ok, rewritten)
	}
	got, found, err := readSessionAttachmentFile(session)
	if err != nil || !found || got != rewritten {
		t.Fatalf("compressed round trip = found:%t content:%q err:%v", found, got, err)
	}
}

func TestPersistSessionFileAttachmentsSkipsOversizedInputAndRejectsOversizedWrite(t *testing.T) {
	session := filepath.Join(t.TempDir(), "rollout.jsonl")
	file, err := os.Create(session)
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
	if got, ok, err := persistSessionFileAttachments(t.TempDir(), session); err != nil || ok || got != "" {
		t.Fatalf("oversized persist = ok:%t content:%q err:%v", ok, got, err)
	}

	original := []byte("unchanged")
	if err := os.WriteFile(session, original, 0o600); err != nil {
		t.Fatal(err)
	}
	tooLarge := strings.Repeat("x", int(sessionAttachmentRewriteMaxBytes)+1)
	if err := writeSessionAttachmentFile(session, tooLarge); err == nil || !strings.Contains(err.Error(), "safe size limit") {
		t.Fatalf("oversized write error = %v", err)
	}
	if got, err := os.ReadFile(session); err != nil || string(got) != string(original) {
		t.Fatalf("oversized write changed original = %q err=%v", got, err)
	}
}

func TestPersistSessionFileAttachmentsRedactsMissingSessionPath(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "secret-profile", "rollout.jsonl")
	_, _, err := persistSessionFileAttachments(t.TempDir(), secretPath)
	if err == nil || err.Error() != "failed to read codex session file" {
		t.Fatalf("missing session error = %v", err)
	}
	if strings.Contains(err.Error(), "secret-profile") || strings.Contains(err.Error(), secretPath) {
		t.Fatalf("session path leaked in error: %v", err)
	}
}

func TestPersistSessionFileAttachmentsPreservesSourceTextWithImagePrefix(t *testing.T) {
	home := t.TempDir()
	sessionDir := filepath.Join(home, "sessions", "2026", "07", "10")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(sessionDir, "rollout.jsonl")
	contents := "{\"timestamp\":\"2026-07-10T00:00:00Z\",\"type\":\"session_meta\",\"payload\":{\"id\":\"test\"}}\n" +
		"{\"type\":\"response_item\",\"payload\":{\"text\":\"const PREFIX: &str = \\\"<image \\\"; value > 0;\"}}\n"
	if err := os.WriteFile(session, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := persistSessionFileAttachments(home, session); err != nil || !ok || got != contents {
		t.Fatalf("source-prefix persist = ok:%t content:%q err:%v", ok, got, err)
	}
	onDisk, err := os.ReadFile(session)
	if err != nil || string(onDisk) != contents {
		t.Fatalf("source-prefix rollout changed = %q err=%v", onDisk, err)
	}
}
