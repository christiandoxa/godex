package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The native macOS/Windows lanes must exercise any new mandatory pinned
// executable mode rather than allowing only Linux release qualification.
func TestProdex04383NativeMatrixRunsPromptCacheOracle(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "verify-native-parity.ps1")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "'--http-prompt-cache'") {
		t.Fatal("native 0.439.0 mode matrix omits the prompt-cache differential")
	}
}
