package update

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestUpdateCacheIsPrivateBoundedAndExpires(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Unix(1000, 0)
	if err := store.SaveLatest("1.2.3", now); err != nil {
		t.Fatal(err)
	}
	if got, ok := store.CachedLatest(now.Add(4 * time.Minute)); !ok || got != "1.2.3" {
		t.Fatalf("cached = %q, %t", got, ok)
	}
	if _, ok := store.CachedLatest(now.Add(5 * time.Minute)); ok {
		t.Fatal("expired cache remained valid")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(root, "update-check.json"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("cache mode = %v, err = %v", info.Mode().Perm(), err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "update-check.json"), make([]byte, cacheMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.CachedLatest(now); ok {
		t.Fatal("oversized cache accepted")
	}
}

func TestUpdateLocksArePersistentAndExclusive(t *testing.T) {
	store := NewStore(t.TempDir())
	release, err := store.AcquireInstall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := store.AcquireInstall(ctx); err == nil {
		t.Fatal("second install lock unexpectedly acquired")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}
