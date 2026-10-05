package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCopyCodexHomeMatchesProdexRootPolicy(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "profile")
	writeCodexTestFile(t, filepath.Join(source, "config.toml"), "model = \"gpt-5\"\n", 0o640)
	writeCodexTestFile(t, filepath.Join(source, "packages", "standalone", "codex"), "installer", 0o700)
	writeCodexTestFile(t, filepath.Join(source, "skills", "packages", "manifest.json"), "{}", 0o600)
	accessed := time.Unix(1_765_926_960, 0)
	modified := time.Unix(1_765_930_560, 0)
	if err := os.Chtimes(filepath.Join(source, "config.toml"), accessed, modified); err != nil {
		t.Fatal(err)
	}

	if err := CopyCodexHome(source, destination); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(destination, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().Unix() != modified.Unix() {
		t.Fatalf("mtime = %v want %v", info.ModTime(), modified)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if got := fileAccessTime(info).Unix(); got != accessed.Unix() {
			t.Fatalf("atime = %d want %d", got, accessed.Unix())
		}
	}
	content, err := os.ReadFile(filepath.Join(destination, "config.toml"))
	if err != nil || string(content) != "model = \"gpt-5\"\n" {
		t.Fatalf("config = %q err=%v", content, err)
	}
	if _, err := os.Lstat(filepath.Join(destination, "packages")); !os.IsNotExist(err) {
		t.Fatalf("root packages unexpectedly copied: %v", err)
	}
	nested, err := os.ReadFile(filepath.Join(destination, "skills", "packages", "manifest.json"))
	if err != nil || string(nested) != "{}" {
		t.Fatalf("nested packages = %q err=%v", nested, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o want 640", info.Mode().Perm())
	}
}

func TestCopyCodexHomeSymlinkPolicyMatchesProdex(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires Windows privileges")
	}
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "profile")
	inside := filepath.Join(source, "inside.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeCodexTestFile(t, inside, "inside", 0o600)
	writeCodexTestFile(t, outside, "outside", 0o600)
	if err := os.Symlink("inside.txt", filepath.Join(source, "inside-link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "escape-link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing.txt", filepath.Join(source, "broken-link.txt")); err != nil {
		t.Fatal(err)
	}

	if err := CopyCodexHome(source, destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "inside-link.txt"))
	if err != nil || string(content) != "inside" {
		t.Fatalf("inside link copy = %q err=%v", content, err)
	}
	for _, name := range []string{"escape-link.txt", "broken-link.txt"} {
		if _, err := os.Lstat(filepath.Join(destination, name)); !os.IsNotExist(err) {
			t.Fatalf("%s unexpectedly copied: %v", name, err)
		}
	}
}

func TestCopyCodexHomeRejectsNonemptyDestinationAndCleansNewFailure(t *testing.T) {
	source := t.TempDir()
	nonempty := t.TempDir()
	writeCodexTestFile(t, filepath.Join(nonempty, "keep"), "keep", 0o600)
	if err := CopyCodexHome(source, nonempty); err == nil {
		t.Fatal("nonempty destination accepted")
	}
	if content, err := os.ReadFile(filepath.Join(nonempty, "keep")); err != nil || string(content) != "keep" {
		t.Fatalf("existing destination changed: %q err=%v", content, err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	failingSource := t.TempDir()
	if err := os.Symlink(".", filepath.Join(failingSource, "directory-link")); err != nil {
		t.Fatal(err)
	}
	newDestination := filepath.Join(t.TempDir(), "new-profile")
	if err := CopyCodexHome(failingSource, newDestination); err == nil {
		t.Fatal("directory symlink unexpectedly copied as file")
	}
	if _, err := os.Stat(newDestination); !os.IsNotExist(err) {
		t.Fatalf("failed new destination was not cleaned: %v", err)
	}

	existingEmpty := filepath.Join(t.TempDir(), "existing-empty")
	if err := os.Mkdir(existingEmpty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := CopyCodexHome(failingSource, existingEmpty); err == nil {
		t.Fatal("directory symlink unexpectedly copied into existing destination")
	}
	if info, err := os.Stat(existingEmpty); err != nil || !info.IsDir() {
		t.Fatalf("pre-existing empty destination was removed: info=%v err=%v", info, err)
	}
}

func TestCopyCodexRegularFileRejectsSymlinkSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires Windows privileges")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "link.txt")
	writeCodexTestFile(t, target, "secret", 0o600)
	if err := os.Symlink("target.txt", link); err != nil {
		t.Fatal(err)
	}
	if err := copyCodexRegularFile(link, filepath.Join(t.TempDir(), "copied.txt")); err == nil {
		t.Fatal("direct regular-file copy followed a symbolic link")
	}
}

func TestCopyCodexHomeRejectsSamePath(t *testing.T) {
	source := t.TempDir()
	if err := CopyCodexHome(source, source); err == nil {
		t.Fatal("same-path copy accepted")
	}
}

func writeCodexTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
