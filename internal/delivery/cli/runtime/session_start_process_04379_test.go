package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// The callback is a separate process in real Codex; in-process goroutines
// alone cannot prove serialization of the private root marker and cursor.
func TestProdex04379NativeMarkerConcurrentProcessesPreserveUnreadFailure(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native callback process fixture qualified on Linux")
	}
	home := t.TempDir()
	godexBinary := filepath.Join(home, "godex")
	repoRoot, err := filepath.Abs("../../../../")
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-trimpath", "-o", godexBinary, "./cmd/godex")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native callback receiver: %v: %s", err, output)
	}
	path := filepath.Join(home, "rollout-2026-10-13-"+markerRoot04379+".jsonl")
	baseline := `{"type":"session_meta","payload":{"id":"` + markerRoot04379 + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(baseline), 0o600); err != nil {
		t.Fatal(err)
	}
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	canonical := fmt.Sprintf(`{"session_id":%q,"transcript_path":%q}`, markerRoot04379, path)
	legacy := `{"thread-id":"00000000-0000-4000-8000-000000000002"}`
	notify := func(ctx context.Context, payload string) error {
		cmd := exec.CommandContext(ctx, godexBinary, sessionStartNotifyCommand04360, marker.path, payload)
		cmd.Env = []string{"HOME=" + home, "CODEX_HOME=" + home, "PATH=" + os.Getenv("PATH")}
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("native callback: %w: %s", err, output)
		}
		return nil
	}
	batch := func(payloads []string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
		defer cancel()
		start := make(chan struct{})
		failures := make(chan error, len(payloads))
		var wg sync.WaitGroup
		for _, payload := range payloads {
			wg.Add(1)
			go func(payload string) {
				defer wg.Done()
				<-start
				failures <- notify(ctx, payload)
			}(payload)
		}
		close(start)
		wg.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	// A legacy descendant can win the very first file lock. The canonical
	// SessionStart must then replace it while all later callbacks are no-ops.
	batch([]string{legacy, legacy, canonical, canonical, legacy, legacy, canonical, legacy})
	if marker.ID() != markerRoot04379 {
		t.Fatalf("root was redirected by descendant processes: %q", marker.ID())
	}
	offset, ok := sessionStartReadOffset04379(t.Context(), marker.path, markerRoot04379)
	if !ok || offset != int64(len(baseline)) {
		t.Fatalf("cursor after subprocess races: offset=%d valid=%t", offset, ok)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(strings.TrimPrefix(markerHistory04379, baseline))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	// Repeated native hooks must not seek past the unread terminal event.
	batch([]string{legacy, canonical, legacy, canonical, legacy, canonical})
	if current, ok := sessionStartReadOffset04379(t.Context(), marker.path, markerRoot04379); !ok || current != offset {
		t.Fatalf("subprocess callbacks reset unread cursor: before=%d after=%d valid=%t", offset, current, ok)
	}
	state, ok := cursorMonitor04379(marker, path).readState(t.Context())
	if !ok || state.Status != "usage_limited" {
		t.Fatalf("subprocess marker races lost unobserved failure: %+v valid=%t", state, ok)
	}
}
