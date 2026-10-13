package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// Two Codex rollout files can have the same session UUID but different
// histories. A cursor captured in one file is unsafe to replay in the other.
func TestProdex04379SessionStartCursorNeverCrossesRolloutPaths(t *testing.T) {
	for _, suffix := range []string{".jsonl", ".jsonl.zst"} {
		t.Run(suffix, func(t *testing.T) {
			root := t.TempDir()
			first := filepath.Join(root, "rollout-2026-10-13-"+markerRoot04379+suffix)
			other := filepath.Join(root, "other", "rollout-2026-10-13-"+markerRoot04379+suffix)
			if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
				t.Fatal(err)
			}
			meta := strings.SplitN(markerHistory04379, "\n", 2)[0] + "\n"
			var encoder *zstd.Encoder
			if suffix == ".jsonl.zst" {
				var err error
				encoder, err = zstd.NewWriter(nil)
				if err != nil {
					t.Fatal(err)
				}
				defer encoder.Close()
			}
			write := func(path, contents string) {
				t.Helper()
				body := []byte(contents)
				if encoder != nil {
					body = encoder.EncodeAll(body, nil)
				}
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(first, meta)
			write(other, markerHistory04379)
			marker, err := newSessionStartMarker04360()
			if err != nil {
				t.Fatal(err)
			}
			defer marker.Close()
			markerNotify04379(t, marker, map[string]any{
				"session_id": markerRoot04379, "transcript_path": first,
			})
			// The failing rollout was already complete before the callback,
			// and must never become new simply because it shares the UUID.
			foreign := cursorMonitor04379(marker, other)
			if state, ok := foreign.readState(t.Context()); !ok || state.Status != "active" {
				t.Fatalf("replayed unrelated rollout from same UUID: state=%+v ok=%t", state, ok)
			}
			// On the originally captured path a new failure remains visible.
			write(first, markerHistory04379)
			owned := cursorMonitor04379(marker, first)
			if state, ok := owned.readState(t.Context()); !ok || state.Status != "usage_limited" {
				t.Fatalf("lost valid native failure on captured rollout: %+v ok=%t", state, ok)
			}
		})
	}
}

func TestProdex04379CursorRolloutSidecarFailClosed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout-2026-10-13-"+markerRoot04379+".jsonl")
	if err := os.WriteFile(path, []byte(markerHistory04379), 0o600); err != nil {
		t.Fatal(err)
	}
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	markerNotify04379(t, marker, map[string]any{
		"session_id": markerRoot04379, "transcript_path": path,
	})
	baseline, ok := sessionStartReadOffsetForRollout04379(t.Context(), marker.path, markerRoot04379, path)
	if !ok || baseline != int64(len(markerHistory04379)) {
		t.Fatalf("cursor was not bound to rollout: offset=%d ok=%t", baseline, ok)
	}
	binding := sessionStartRolloutSidecarPath04379(marker.path)
	for _, data := range []string{"/elsewhere/rollout.jsonl\n", "relative.jsonl\n", "", strings.Repeat("x", 5000)} {
		if err := os.WriteFile(binding, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := sessionStartReadOffsetForRollout04379(t.Context(), marker.path, markerRoot04379, path); ok {
			t.Fatalf("accepted forged sidecar data %q", data[:min(len(data), 60)])
		}
	}
	if err := os.Remove(binding); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, binding); err == nil {
		if _, ok := sessionStartReadOffsetForRollout04379(t.Context(), marker.path, markerRoot04379, path); ok {
			t.Fatal("accepted symlinked rollout sidecar")
		}
	}
	// Explicit new root without a transcript must remove both sidecars.
	markerNotify04379(t, marker, map[string]any{"session_id": "00000000-0000-4000-8000-000000000099"})
	for _, sidecar := range []string{binding, sessionStartOffsetPath04379(marker.path)} {
		if _, err := os.Lstat(sidecar); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retained old session state after root transition %s: %v", sidecar, err)
		}
	}
}
