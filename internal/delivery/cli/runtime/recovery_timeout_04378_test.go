package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const recovery04378Session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"

// Prodex 0.437.8 source oracle: mojo/prodex_core/launch_config.mojo.
// Only exact native timeout text with the typed other variant, plus an
// absent/408/504 HTTP status, grants terminal transport recovery.
func TestProdex04378NativeTimeoutWorkflowClassifier(t *testing.T) {
	tests := []struct {
		name, message, variant, completionStatus, want string
		httpStatus                                     int
		wrapped                                        bool
		appServer                                      bool
	}{
		{"native terminal timeout", "request timed out", "other", "", "transport", 0, true, false},
		{"remote compact timeout", "Error running remote compact task: request timed out", "other", "", "transport", 0, true, false},
		{"unwrapped native alias", "request timed out", "other", "", "transport", 0, false, false},
		{"HTTP 408", "request timed out", "other", "", "transport", 408, false, false},
		{"HTTP 504", "request timed out", "other", "", "transport", 504, false, false},
		{"HTTP 400 forbidden", "request timed out", "other", "", "", 400, false, false},
		{"HTTP 401 forbidden", "request timed out", "other", "", "", 401, false, false},
		{"HTTP 429 forbidden", "request timed out", "other", "", "", 429, false, false},
		{"unrelated error union", "request timed out", "contextWindowExceeded", "", "", 0, false, false},
		{"quoted timeout not authority", "a tool reported request timed out", "other", "", "", 0, false, false},
		{"suffix not authority", "request timed out; rerun all tools", "other", "", "", 0, false, false},
		{"no typed variant", "request timed out", "", "", "", 0, false, false},
		{"success", "request timed out", "other", "completed", "", 0, false, false},
		{"cancelled", "request timed out", "other", "cancelled", "", 0, false, false},
		{"interrupted", "request timed out", "other", "interrupted", "", 0, false, false},
		{"app-server success", "request timed out", "other", "completed", "", 0, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			failure := map[string]any{"message": tc.message}
			if tc.variant != "" {
				failure["codex_error_info"] = tc.variant
			}
			if tc.httpStatus > 0 {
				failure["http_status_code"] = tc.httpStatus
			}
			completion := map[string]any{"type": "task_complete", "error": failure}
			if tc.completionStatus != "" {
				completion["status"] = tc.completionStatus
			}
			var record map[string]any
			switch {
			case tc.appServer:
				record = map[string]any{"method": "turn/completed", "params": map[string]any{"turn": completion}}
			case tc.wrapped:
				record = map[string]any{"type": "event_msg", "payload": completion}
			default:
				completion["type"] = "turn_complete"
				record = completion
			}
			// Real rollout JSON decoding produces float64 numbers; never
			// bypass the production parser by passing a Go-native int.
			wire, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(wire, &decoded); err != nil {
				t.Fatal(err)
			}
			if got := structuredWorkflowRecoveryClass04360(decoded); got != tc.want {
				bytes, _ := json.Marshal(record)
				t.Fatalf("native recovery=%q want %q; record=%s", got, tc.want, bytes)
			}
		})
	}
}

func TestProdex04378NativeTimeoutPostChildRolloutRecovery(t *testing.T) {
	cases := []struct {
		name    string
		records []map[string]any
		want    string
	}{
		{"accepted timeout", []map[string]any{nativeTimeoutRecord04378("request timed out")}, "transport"},
		{"accepted compact timeout", []map[string]any{nativeTimeoutRecord04378("Error running remote compact task: request timed out")}, "transport"},
		{"foreign terminal timeout", []map[string]any{foreignNativeTimeoutRecord04378()}, ""},
		{"new turn without acceptance", []map[string]any{{"type": "task_started"}, nativeTimeoutRecord04378("request timed out")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout-"+recovery04378Session+".jsonl")
			initial, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]any{"id": recovery04378Session}})
			if err := os.WriteFile(path, append(initial, 10), 0600); err != nil {
				t.Fatal(err)
			}
			checkpoint := captureRecoveryCheckpoint04360(path)
			if !checkpoint.valid {
				t.Fatal("invalid session checkpoint")
			}
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			records := append([]map[string]any{{"type": "response_item", "payload": map[string]any{"role": "user"}}}, tc.records...)
			for _, record := range records {
				wire, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = file.Write(append(wire, 10)); err != nil {
					t.Fatal(err)
				}
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if got := checkpoint.newAcceptedRecoveryClass04360(context.Background(), recovery04378Session); got != tc.want {
				t.Fatalf("post-child native timeout class=%q want %q", got, tc.want)
			}
		})
	}
}

func nativeTimeoutRecord04378(message string) map[string]any {
	return map[string]any{"type": "event_msg", "payload": map[string]any{"type": "task_complete", "error": map[string]any{"message": message, "codex_error_info": "other"}}}
}
func foreignNativeTimeoutRecord04378() map[string]any {
	return map[string]any{"type": "task_complete", "session_id": "00000000-0000-4000-8000-000000000002", "error": map[string]any{"message": "request timed out", "codex_error_info": "other"}}
}

// The post-child recovery scan and continuously running live monitor have
// different evidence windows. Only live recovery must drain all later native
// activity before arming a pending recovery.
func TestProdex04378NativeTimeoutLiveTailSupersession(t *testing.T) {
	for _, tc := range []struct{ name, next string }{
		{"progress after timeout", "agent_message"},
		{"cancel after timeout", "turn_cancelled"},
		{"successful completion after timeout", "turn_complete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := []map[string]any{
				nativeTimeoutRecord04378("request timed out"),
			}
			next := map[string]any{"type": "event_msg", "payload": map[string]any{"type": tc.next}}
			if tc.next == "turn_complete" {
				next = map[string]any{"type": "turn_complete", "error": nil}
			}
			records = append(records, next)
			var tail []byte
			for _, record := range records {
				wire, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				tail = append(tail, wire...)
				tail = append(tail, 10)
			}
			got, _, _, _ := scanRecoveryClassSince04377(context.Background(), recovery04378Session, tail, true)
			if got != "" {
				t.Fatalf("newer %s must suppress stale terminal timeout, got=%q", tc.next, got)
			}
		})
	}
}
