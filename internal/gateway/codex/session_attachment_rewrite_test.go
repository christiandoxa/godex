package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRewriteSessionPersistedAttachmentPathsCopiesStableFiles(t *testing.T) {
	home := t.TempDir()
	imageSource := filepath.Join(t.TempDir(), "codex-clipboard-a.png")
	attachmentSource := filepath.Join(t.TempDir(), "overlay", "attachments", "thread-a", "pasted-text-1.txt")
	if err := os.WriteFile(imageSource, []byte("image-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(attachmentSource), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(attachmentSource, []byte("pasted-text"), 0o600); err != nil {
		t.Fatal(err)
	}
	contents := `<image path="` + imageSource + `"> ` + attachmentSource
	rewritten, err := rewriteSessionPersistedAttachmentPaths(home, contents)
	if err != nil {
		t.Fatal(err)
	}
	stableImage := filepath.Join(home, "image_attachments", filepath.Base(imageSource))
	stableAttachment := filepath.Join(home, "attachments", "thread-a", "pasted-text-1.txt")
	if !strings.Contains(rewritten, stableImage) || !strings.Contains(rewritten, stableAttachment) ||
		strings.Contains(rewritten, imageSource) || strings.Contains(rewritten, attachmentSource) {
		t.Fatalf("rewritten = %q", rewritten)
	}
	if got, err := os.ReadFile(stableImage); err != nil || string(got) != "image-bytes" {
		t.Fatalf("stable image = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(stableAttachment); err != nil || string(got) != "pasted-text" {
		t.Fatalf("stable attachment = %q, err=%v", got, err)
	}
}

func TestRewriteSessionPersistedAttachmentPathsUsesExistingStableCopyWhenSourceGone(t *testing.T) {
	home := t.TempDir()
	stable := filepath.Join(home, "image_attachments", "codex-clipboard-gone.png")
	if err := os.MkdirAll(filepath.Dir(stable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stable, []byte("stable"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "deleted", filepath.Base(stable))
	contents := `<image path="` + missing + `">`
	rewritten, err := rewriteSessionPersistedAttachmentPaths(home, contents)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rewritten, stable) || strings.Contains(rewritten, missing) {
		t.Fatalf("rewritten = %q", rewritten)
	}
}

func TestRewriteSessionAttachmentRejectsSymlinkSourceAndReplacesStableSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior is covered on Unix")
	}
	home := t.TempDir()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkSource := filepath.Join(root, "overlay", "attachments", "id-a", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(symlinkSource), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, symlinkSource); err != nil {
		t.Fatal(err)
	}
	rewritten, err := rewriteSessionPersistedAttachmentPaths(home, symlinkSource)
	if err != nil {
		t.Fatal(err)
	}
	if rewritten != symlinkSource {
		t.Fatalf("symlink source rewritten to %q", rewritten)
	}

	source := filepath.Join(root, "overlay", "attachments", "id-b", "pasted-text-1.txt")
	stable := filepath.Join(home, "attachments", "id-b", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(stable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, stable); err != nil {
		t.Fatal(err)
	}
	rewritten, err = rewriteSessionPersistedAttachmentPaths(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if rewritten != stable {
		t.Fatalf("stable symlink replacement path = %q, want %q", rewritten, stable)
	}
	info, err := os.Lstat(stable)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("stable replacement info = %#v, err=%v", info, err)
	}
	if got, _ := os.ReadFile(target); string(got) != "outside" {
		t.Fatalf("outside target changed to %q", got)
	}
}

func TestSessionAttachmentPathSuffixMatchesTaggedAllowlist(t *testing.T) {
	valid := filepath.Join(string(filepath.Separator), "tmp", "attachments", "thread-a", "image-1.png")
	if got, ok := sessionAttachmentPathSuffix(valid); !ok || got != filepath.Join("thread-a", "image-1.png") {
		t.Fatalf("valid suffix = %q, %t", got, ok)
	}
	for _, path := range []string{
		filepath.Join(string(filepath.Separator), "tmp", "attachments", "thread-a", "other.txt"),
		filepath.Join(string(filepath.Separator), "tmp", "attachments", "thread-a", "pasted-text-1.txt", "extra"),
		filepath.Join(string(filepath.Separator), "tmp", "attachments", "pasted-text-1.txt"),
	} {
		if got, ok := sessionAttachmentPathSuffix(path); ok {
			t.Errorf("invalid suffix %q accepted as %q", path, got)
		}
	}
}

func TestSessionClipboardSourcePersistableIsTempRootBounded(t *testing.T) {
	inside := filepath.Join(os.TempDir(), "nested", "codex-clipboard-a.png")
	if !sessionClipboardSourcePersistable(inside) {
		t.Fatalf("temp source rejected: %s", inside)
	}
	outside := filepath.Join(filepath.Dir(filepath.Clean(os.TempDir())), "definitely-outside-temp", "codex-clipboard-a.png")
	if filepath.Clean(outside) != filepath.Clean(inside) && sessionClipboardSourcePersistable(outside) {
		t.Fatalf("outside source accepted: %s", outside)
	}
}

func TestRewriteSessionAttachmentPreservesSourceModeAndModifiedTime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertion is Unix-specific")
	}
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "overlay", "attachments", "id-mode", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("mode-and-time"), 0o640); err != nil {
		t.Fatal(err)
	}
	modified := time.Unix(1_700_000_456, 789_000_000)
	if err := os.Chtimes(source, modified.Add(-time.Hour), modified); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(home, "attachments", "id-mode", "pasted-text-1.txt")
	if rewritten, err := rewriteSessionPersistedAttachmentPaths(home, source); err != nil || rewritten != stable {
		t.Fatalf("rewrite = %q, err=%v", rewritten, err)
	}
	info, err := os.Stat(stable)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 || !info.ModTime().Equal(modified) {
		t.Fatalf("stable metadata = mode:%o mtime:%s", info.Mode().Perm(), info.ModTime())
	}
}

func TestProdex04356AttachmentScannerNeverRewindsAcrossEscapedNewline(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	firstID := "11111111-2222-4333-8444-555555555555"
	secondID := "66666666-7777-4888-8999-aaaaaaaaaaaa"
	oldFirst := filepath.Join(root, "deleted-overlay", "attachments", firstID, "pasted-text-1.txt")
	oldSecond := filepath.Join(root, "deleted-overlay", "attachments", secondID, "image-1.png")
	stableFirst := filepath.Join(home, "attachments", firstID, "pasted-text-1.txt")
	stableSecond := filepath.Join(home, "attachments", secondID, "image-1.png")
	for path, contents := range map[string]string{
		stableFirst:  "stable text",
		stableSecond: "stable image",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	payload := `{"type":"response_item","payload":{"text":` +
		strconv.Quote(oldFirst+"\n"+oldSecond) + `}}`
	rewritten, err := rewriteSessionPersistedAttachmentPaths(home, payload)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(rewritten), &value); err != nil {
		t.Fatalf("rewritten session is invalid JSON: %v body=%s", err, rewritten)
	}
	nested, _ := value["payload"].(map[string]any)
	text, _ := nested["text"].(string)
	if !strings.Contains(text, stableFirst) || !strings.Contains(text, stableSecond) ||
		strings.Contains(text, "deleted-overlay") {
		t.Fatalf("rewritten attachment text = %q", text)
	}
}
