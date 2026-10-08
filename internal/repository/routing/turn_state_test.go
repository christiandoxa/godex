package routing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResponseTurnStatePersistsPrivatelyAndExpires(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir())
	key := strings.Repeat("a", 64)
	turnState := "opaque-continuation-state"
	expiresAt := time.Unix(10_000, 0)
	if err := store.SaveResponseTurnState(context.Background(), home, key, turnState, expiresAt); err != nil {
		t.Fatal(err)
	}

	got, expires, err := store.LoadResponseTurnState(context.Background(), home, key, expiresAt.Add(-time.Second))
	if err != nil || got != turnState || !expires.Equal(expiresAt) {
		t.Fatalf("loaded turn state = %q, %v, %v", got, expires, err)
	}
	if got, _, err := store.LoadResponseTurnState(context.Background(), home, key, expiresAt); err != nil || got != "" {
		t.Fatalf("expired turn state = %q, %v", got, err)
	}

	path := filepath.Join(home, responseTurnStateDirectory, key+".json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), key) || !strings.Contains(string(content), turnState) {
		t.Fatal("turn-state file did not keep the opaque value separate from its hashed filename")
	}
	if _, err := os.Stat(filepath.Join(home, "routing.json")); !os.IsNotExist(err) {
		t.Fatalf("turn-state sidecar created global routing metadata: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("turn-state file permissions = %v, %v", info, err)
		}
		info, err = os.Stat(filepath.Dir(path))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("turn-state directory permissions = %v, %v", info, err)
		}
	}
}

func TestResponseTurnStateSaveRecoversOrphanedAtomicWriteAtCapacity(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(home, responseTurnStateDirectory)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := range responseTurnStateFiles {
		name := fmt.Sprintf("%064x.json", i)
		if err := os.WriteFile(filepath.Join(directory, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	orphan := filepath.Join(directory, ".atomic-crash-leftover")
	if err := os.WriteFile(orphan, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	key := strings.Repeat("f", 64)
	store := NewStore(t.TempDir())
	if err := store.SaveResponseTurnState(context.Background(), home, key, "recovered-save", time.Unix(10_000, 0)); err != nil {
		t.Fatalf("save after interrupted atomic write: %v", err)
	}
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphaned atomic file remains: %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	stateFiles := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			stateFiles++
		}
	}
	if stateFiles != responseTurnStateFiles {
		t.Fatalf("turn-state file count = %d, want %d", stateFiles, responseTurnStateFiles)
	}
	if got, _, err := store.LoadResponseTurnState(context.Background(), home, key, time.Unix(9_999, 0)); err != nil || got != "recovered-save" {
		t.Fatalf("saved state = %q, %v", got, err)
	}
}

func TestResponseTurnStateRemovalDeletesOnlyHashedSidecar(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir())
	key := strings.Repeat("c", 64)
	expiresAt := time.Unix(10_000, 0)
	if err := store.SaveResponseTurnState(context.Background(), home, key, "opaque-state", expiresAt); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveResponseTurnState(context.Background(), home, key); err != nil {
		t.Fatal(err)
	}
	if value, _, err := store.LoadResponseTurnState(context.Background(), home, key, expiresAt); err != nil || value != "" {
		t.Fatalf("removed turn state = %q, %v", value, err)
	}
}

func TestResponseTurnStateDoesNotPersistInAnInsecureProfileHome(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Skip("profile permission bits are not enforced on Windows")
	}
	if err := os.Chmod(home, 0o755); err != nil {
		t.Fatal(err)
	}
	err := NewStore(t.TempDir()).SaveResponseTurnState(
		context.Background(), home, strings.Repeat("b", 64), "opaque-state", time.Now().Add(time.Minute),
	)
	if err == nil {
		t.Fatal("persisted turn state in a profile home accessible to other users")
	}
}
