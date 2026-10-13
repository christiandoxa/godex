package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	"github.com/klauspost/compress/zstd"
)

const markerRoot04379 = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
const markerHistory04379 = `{"type":"session_meta","payload":{"id":"` + markerRoot04379 + `"}}` + "\n" +
	`{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
	`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n"

func markerNotify04379(t *testing.T, marker *sessionStartMarker04360, fields map[string]any) {
	t.Helper()
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	handled, err := HandleSessionStartNotify04360(
		[]string{sessionStartNotifyCommand04360, marker.path, string(data)}, nil,
	)
	if !handled || err != nil {
		t.Fatalf("SessionStart callback %v: handled=%t error=%v", fields, handled, err)
	}
}

func cursorMonitor04379(marker *sessionStartMarker04360, path string) *liveGoalRecovery04361 {
	return &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: markerRoot04379, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: markerRoot04379, Path: path}, nil
		},
	}
}

func TestProdex04379CallbackCursorRecoversUnseenTerminalAfterStart(t *testing.T) {
	initial := `{"type":"session_meta","payload":{"id":"` + markerRoot04379 + `"}}` + "\n"
	for _, suffix := range []string{".jsonl", ".jsonl.zst"} {
		t.Run(suffix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout-2026-10-13-"+markerRoot04379+suffix)
			var encoder *zstd.Encoder
			if suffix == ".jsonl.zst" {
				var err error
				encoder, err = zstd.NewWriter(nil)
				if err != nil {
					t.Fatal(err)
				}
				defer encoder.Close()
			}
			write := func(body string) {
				t.Helper()
				data := []byte(body)
				if encoder != nil {
					data = encoder.EncodeAll(data, nil)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(initial)
			marker, err := newSessionStartMarker04360()
			if err != nil {
				t.Fatal(err)
			}
			defer marker.Close()
			markerNotify04379(t, marker, map[string]any{
				"session_id": markerRoot04379, "transcript_path": path,
			})
			offset, err := os.ReadFile(sessionStartOffsetPath04379(marker.path))
			if err != nil || string(offset) != strconv.Itoa(len(initial))+"\n" {
				t.Fatalf("missing decoded cursor: offset=%q error=%v", offset, err)
			}
			// The native terminal failure arrives before any monitor callback.
			// Compressed rollouts are atomically rewritten, not physically appended.
			write(markerHistory04379)
			monitor := cursorMonitor04379(marker, path)
			state, ok := monitor.readState(t.Context())
			if !ok || state.Status != "usage_limited" {
				t.Fatalf("failure before first monitor poll was lost: state=%+v ok=%t", state, ok)
			}
		})
	}
}

func TestProdex04379CursorNeverReplaysHistoricalFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-10-13-"+markerRoot04379+".jsonl")
	if err := os.WriteFile(path, []byte(markerHistory04379), 0o600); err != nil {
		t.Fatal(err)
	}
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	markerNotify04379(t, marker, map[string]any{"session_id": markerRoot04379, "transcript_path": path})
	monitor := cursorMonitor04379(marker, path)
	if state, ok := monitor.readState(t.Context()); !ok || state.Status != "active" {
		t.Fatalf("historical terminal error was replayed: %+v ok=%t", state, ok)
	}
}

func TestProdex04379RootCallbacksPreserveCursorAndClearOnNewSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-10-13-"+markerRoot04379+".jsonl")
	if err := os.WriteFile(path, []byte(markerHistory04379), 0o600); err != nil {
		t.Fatal(err)
	}
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	markerNotify04379(t, marker, map[string]any{"session_id": markerRoot04379, "transcript_path": path})
	offsetPath := sessionStartOffsetPath04379(marker.path)
	first, err := os.ReadFile(offsetPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, fields := range []map[string]any{
		{"thread-id": "00000000-0000-4000-8000-000000000002"},
		{"session_id": markerRoot04379, "source": "compact"},
		{"thread-id": markerRoot04379},
	} {
		markerNotify04379(t, marker, fields)
		value, err := os.ReadFile(offsetPath)
		if err != nil || string(value) != string(first) || marker.ID() != markerRoot04379 {
			t.Fatalf("legacy/repeated hook changed cursor or root: got=%q root=%q err=%v", value, marker.ID(), err)
		}
	}
	nextID := "00000000-0000-4000-8000-000000000003"
	markerNotify04379(t, marker, map[string]any{"session_id": nextID})
	if marker.ID() != nextID {
		t.Fatalf("new root missing: %q", marker.ID())
	}
	if _, err := os.Lstat(offsetPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale offset retained after explicit new root: %v", err)
	}
}

func TestProdex04379CursorRejectsForeignOrMalformedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pathName string
		body     string
		symlink  bool
	}{
		{"foreign_metadata", markerRoot04379, strings.ReplaceAll(markerHistory04379, markerRoot04379, "00000000-0000-4000-8000-000000000005"), false},
		{"foreign_filename", "00000000-0000-4000-8000-000000000005", markerHistory04379, false},
		{"symlink", markerRoot04379, markerHistory04379, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "rollout-2026-10-13-"+tc.pathName+".jsonl")
			if tc.symlink {
				target := filepath.Join(root, "elsewhere.jsonl")
				if err := os.WriteFile(target, []byte(tc.body), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skip(err)
				}
			} else if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			marker, err := newSessionStartMarker04360()
			if err != nil {
				t.Fatal(err)
			}
			defer marker.Close()
			markerNotify04379(t, marker, map[string]any{"session_id": markerRoot04379, "transcript_path": path})
			if _, err := os.Lstat(sessionStartOffsetPath04379(marker.path)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("untrusted transcript established a cursor: %v", err)
			}
		})
	}
}

func TestProdex04379InvalidCursorFallsBackWithoutReplayingHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-10-13-"+markerRoot04379+".jsonl")
	if err := os.WriteFile(path, []byte(markerHistory04379), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"-1\n", "junk\n", "1\n", "999999\n", strings.Repeat("9", 30) + "\n"} {
		t.Run(strings.TrimSpace(value), func(t *testing.T) {
			marker, err := newSessionStartMarker04360()
			if err != nil {
				t.Fatal(err)
			}
			defer marker.Close()
			markerNotify04379(t, marker, map[string]any{"session_id": markerRoot04379, "transcript_path": path})
			if err := os.WriteFile(sessionStartOffsetPath04379(marker.path), []byte(value), 0o600); err != nil {
				t.Fatal(err)
			}
			if state, ok := cursorMonitor04379(marker, path).readState(t.Context()); !ok || state.Status != "active" {
				t.Fatalf("invalid cursor %q replayed history: state=%+v ok=%t", value, state, ok)
			}
		})
	}
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	markerNotify04379(t, marker, map[string]any{"session_id": markerRoot04379})
	linkTarget := filepath.Join(t.TempDir(), "unsafe-offset")
	if err := os.WriteFile(linkTarget, []byte("0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkTarget, sessionStartOffsetPath04379(marker.path)); err == nil {
		if _, ok := sessionStartReadOffset04379(t.Context(), marker.path, markerRoot04379); ok {
			t.Fatal("symlinked cursor accepted")
		}
	}
	if _, ok := sessionStartReadOffset04379(t.Context(), marker.path, "00000000-0000-4000-8000-000000000099"); ok {
		t.Fatal("cursor accepted for a different root")
	}
}
