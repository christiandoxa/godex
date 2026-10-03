package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepairSessionIndexStopsOnMaintenanceFailureAndEmitsTaggedTiming(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "sessions"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRODEX_RUNTIME_TIMINGS", "")
	var stderr bytes.Buffer
	process := NewCodexProcess(filepath.Join(root, "missing-codex"), Terminal{Stderr: &stderr})
	err := process.RepairSessionIndex(context.Background(), filepath.Join(root, "active"), shared, filepath.Join(root, "godex"))
	if err == nil || !strings.Contains(err.Error(), "full session index repair failed") ||
		!strings.Contains(err.Error(), "Codex sessions directory") {
		t.Fatalf("full repair error = %v", err)
	}
	if !strings.Contains(stderr.String(), "prodex_runtime_timing stage=startup.thread_index_reconcile_ms duration_ms=") {
		t.Fatalf("runtime timing = %q", stderr.String())
	}
}
