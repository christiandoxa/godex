package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteReplacesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot")
	for _, content := range []string{"old", "new"} {
		committed, err := AtomicWrite(path, []byte(content))
		if err != nil || !committed {
			t.Fatalf("write = %v, %v", committed, err)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "new" {
		t.Fatalf("content = %q, %v", content, err)
	}
}
