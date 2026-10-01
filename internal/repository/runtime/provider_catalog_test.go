package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProviderCatalogStoreWritesReadsAndSecuresFiles(t *testing.T) {
	home := t.TempDir()
	store := NewProviderCatalogStore()
	models := []map[string]any{{"id": "model-a", "context_window": float64(123)}}
	path, err := store.WriteCopilotRuntime(home, models)
	if err != nil || path != filepath.Join(home, proxymodel.CopilotRuntimeCatalogFile) {
		t.Fatalf("path=%q err=%v", path, err)
	}
	read, err := store.ReadCopilotRuntime(home)
	if err != nil || len(read) != 1 || read[0]["id"] != "model-a" {
		t.Fatalf("read=%#v err=%v", read, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
		}
	}
}

func TestProviderCatalogStoreReplacesSymlinkWithoutTouchingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require privileges on Windows")
	}
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, proxymodel.ExternalProviderCatalogFile)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProviderCatalogStore().WriteExternal(home, []map[string]any{{"id": "model-a"}}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "keep" {
		t.Fatalf("symlink target=%q err=%v", content, err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("replacement info=%v err=%v", info.Mode(), err)
	}
}

func TestProviderCatalogStoreRejectsOversizedCatalog(t *testing.T) {
	home := t.TempDir()
	models := make([]map[string]any, proxymodel.ProviderCatalogMaxItems+1)
	for index := range models {
		models[index] = map[string]any{"id": "model"}
	}
	if _, err := NewProviderCatalogStore().WriteExternal(home, models); err == nil {
		t.Fatal("oversized provider catalog unexpectedly accepted")
	}
}
