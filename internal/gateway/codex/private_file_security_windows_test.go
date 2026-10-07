//go:build windows

package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWritePrivateFileCreatesTrustedWindowsACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.json")
	if err := writePrivateFile(path, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !importCurrentPrivateFileTrusted(path, info) {
		t.Fatal("writePrivateFile did not create a protected current-user ACL")
	}
}
