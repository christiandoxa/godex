package routing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResponseTurnStateRejectsPartialFileWithoutReturningIt(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("d", 64)
	directory := filepath.Join(home, responseTurnStateDirectory)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, key+".json")
	partial := `{"version":1,"expires_unix":`
	if err := os.WriteFile(path, []byte(partial), 0o600); err != nil {
		t.Fatal(err)
	}
	if value, _, err := NewStore(t.TempDir()).LoadResponseTurnState(
		context.Background(), home, key, time.Unix(10_000, 0),
	); err == nil || value != "" {
		t.Fatalf("partial turn state value/error = %q/%v", value, err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != partial {
		t.Fatalf("partial turn-state file changed to %q, error = %v", content, err)
	}
}

func TestResponseTurnStateSaveRemovesOrphanedAtomicWriteAtCapacity(t *testing.T) {
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
}
