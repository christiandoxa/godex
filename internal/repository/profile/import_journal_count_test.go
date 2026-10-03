package profile

import (
	"os"
	"testing"
)

func TestCountImportAuthJournalsIsNonMutating(t *testing.T) {
	store := NewStore(t.TempDir())
	if count, err := store.CountImportAuthJournals(t.Context()); err != nil || count != 0 {
		t.Fatalf("empty count = %d, err=%v", count, err)
	}
	if err := os.WriteFile(store.importAuthJournalPath(), []byte("not-valid-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if count, err := store.CountImportAuthJournals(t.Context()); err != nil || count != 1 {
		t.Fatalf("journal count = %d, err=%v", count, err)
	}
	if _, err := os.Lstat(store.importAuthJournalPath()); err != nil {
		t.Fatalf("count mutated journal: %v", err)
	}
}
