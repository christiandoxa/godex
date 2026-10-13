package runtime

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// A native callback can run while Codex is appending one JSONL event. The
// callback cursor must point to the last complete event, otherwise the first
// monitor scan can discard a real terminal failure that arrived afterwards.
func TestProdex04379SessionStartCursorHandlesPartialRolloutLine(t *testing.T) {
	meta := strings.SplitN(markerHistory04379, "\n", 2)[0] + "\n"
	partial := meta + `{"type":"event_msg","payload":{"type":"user_`
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
			write := func(data string) {
				t.Helper()
				body := []byte(data)
				if encoder != nil {
					body = encoder.EncodeAll(body, nil)
				}
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(partial)
			marker, err := newSessionStartMarker04360()
			if err != nil {
				t.Fatal(err)
			}
			defer marker.Close()
			markerNotify04379(t, marker, map[string]any{
				"session_id": markerRoot04379, "transcript_path": path,
			})
			cursor, err := os.ReadFile(sessionStartOffsetPath04379(marker.path))
			if err != nil || string(cursor) != strconv.Itoa(len(meta))+"\n" {
				t.Fatalf("cursor must stop at last complete native JSONL record: got=%q err=%v want=%d",
					cursor, err, len(meta))
			}

			// Codex finishes the pending user event and writes a capacity
			// error before the live monitor's very first poll.
			if encoder != nil {
				write(markerHistory04379)
			} else {
				file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
				if err != nil {
					t.Fatal(err)
				}
				_, err = file.WriteString(strings.TrimPrefix(markerHistory04379, partial))
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			state, valid := cursorMonitor04379(marker, path).readState(t.Context())
			if !valid || state.Status != "usage_limited" {
				t.Fatalf("pre-poll failure was skipped after a partial SessionStart snapshot: %+v valid=%t",
					state, valid)
			}
		})
	}
}

func TestProdex04379SessionStartCursorRejectsNoCompletedRecord(t *testing.T) {
	for _, suffix := range []string{".jsonl", ".jsonl.zst"} {
		t.Run(suffix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout-2026-10-13-"+markerRoot04379+suffix)
			data := []byte(`{"type":"session_meta","payload":{"id":"` + markerRoot04379)
			if suffix == ".jsonl.zst" {
				encoder, err := zstd.NewWriter(nil)
				if err != nil {
					t.Fatal(err)
				}
				data = encoder.EncodeAll(data, nil)
				encoder.Close()
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
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
			if _, err := os.Lstat(sessionStartOffsetPath04379(marker.path)); !os.IsNotExist(err) {
				t.Fatalf("incomplete metadata must not establish a trusted cursor: %v", err)
			}
		})
	}
}

func TestProdex04379JSONLCursorBackscanIsBounded(t *testing.T) {
	const complete = "{\"type\":\"session_meta\"}\n"
	for _, tc := range []struct {
		name string
		body string
		want int64
	}{
		{"empty", "", -1},
		{"unterminated metadata", "not-yet-complete", -1},
		{"complete last line", complete, int64(len(complete))},
		{"last complete before partial", complete + "{\"partial\":", int64(len(complete))},
		{"large incomplete tail", complete + strings.Repeat("x", (1<<20)+1), -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout.jsonl")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if got := sessionStartLastCompleteJSONLOffset04379(file, int64(len(tc.body))); got != tc.want {
				t.Fatalf("last complete offset = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestProdex04379PartialCursorDoesNotReplayCompletedHistoricalFailure(t *testing.T) {
	partial := `{"type":"event_msg","payload":{"type":"user_`
	path := filepath.Join(t.TempDir(), "rollout-2026-10-13-"+markerRoot04379+".jsonl")
	if err := os.WriteFile(path, []byte(markerHistory04379+partial), 0o600); err != nil {
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
	offset, err := os.ReadFile(sessionStartOffsetPath04379(marker.path))
	if err != nil || string(offset) != strconv.Itoa(len(markerHistory04379))+"\n" {
		t.Fatalf("historical capacity error must stay behind cursor: offset=%q err=%v", offset, err)
	}
	// Another callback from the same root must not reset the saved offset.
	markerNotify04379(t, marker, map[string]any{
		"session_id": markerRoot04379, "source": "compact",
	})
	next, err := os.ReadFile(sessionStartOffsetPath04379(marker.path))
	if err != nil || string(next) != string(offset) {
		t.Fatalf("same-root compact callback modified unread cursor: next=%q err=%v", next, err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("message\"}}\n")
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	state, valid := cursorMonitor04379(marker, path).readState(t.Context())
	if !valid || state.Status != "active" {
		t.Fatalf("replayed historical failure after completion of a new partial event: %+v valid=%t",
			state, valid)
	}
}
