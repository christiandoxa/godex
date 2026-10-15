package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const prodex043713WebSocketIdleMessage = "stream disconnected before completion: idle timeout waiting for websocket"

// Tagged Prodex 0.437.13's transport classifier may recover only a terminal
// native error with the precise WebSocket idle message and typed "other" info.
// Similar user prose, client errors, successes and cancellations never authorize replay.
func TestProdex043713WebSocketIdleTerminalRecoveryClassification(t *testing.T) {
	cases := []struct {
		name   string
		record map[string]any
		want   string
	}{
		{"native terminal", map[string]any{"type": "event_msg", "payload": map[string]any{"type": "task_complete", "error": map[string]any{"codex_error_info": "other", "message": prodex043713WebSocketIdleMessage}}}, "transport"},
		{"appserver terminal", map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"status": "failed", "error": map[string]any{"codexErrorInfo": "other", "message": prodex043713WebSocketIdleMessage}}}}, "transport"},
		{"native interrupted", map[string]any{"type": "task_complete", "status": "cancelled", "error": map[string]any{"codex_error_info": "other", "message": prodex043713WebSocketIdleMessage}}, ""},
		{"native finished", map[string]any{"type": "task_complete", "status": "completed", "error": map[string]any{"codex_error_info": "other", "message": prodex043713WebSocketIdleMessage}}, ""},
		{"client HTTP 400", map[string]any{"type": "task_complete", "error": map[string]any{"codex_error_info": "other", "http_status_code": 400, "message": prodex043713WebSocketIdleMessage}}, ""},
		{"native HTTP 408", map[string]any{"type": "task_complete", "error": map[string]any{"codex_error_info": "other", "http_status_code": 408, "message": prodex043713WebSocketIdleMessage}}, "transport"},
		{"native HTTP 429", map[string]any{"type": "task_complete", "error": map[string]any{"codex_error_info": "other", "http_status_code": 429, "message": prodex043713WebSocketIdleMessage}}, ""},
		{"other timeout", map[string]any{"type": "task_complete", "error": map[string]any{"codex_error_info": "other", "message": "idle timeout waiting for websocket"}}, ""},
		{"quoted user text", map[string]any{"type": "response_item", "payload": map[string]any{"role": "user", "content": prodex043713WebSocketIdleMessage}}, ""},
		{"untyped failure", map[string]any{"type": "task_complete", "error": map[string]any{"message": prodex043713WebSocketIdleMessage}}, ""},
		{"auth error", map[string]any{"type": "task_complete", "error": map[string]any{"codex_error_info": "unauthorized", "message": prodex043713WebSocketIdleMessage}}, "auth"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := json.Marshal(tc.record)
			if err != nil {
				t.Fatal(err)
			}
			var parsed map[string]any
			if err := json.Unmarshal(wire, &parsed); err != nil {
				t.Fatal(err)
			}
			if got := structuredWorkflowRecoveryClass04360(parsed); got != tc.want {
				t.Fatalf("class=%q, want=%q, record=%s", got, tc.want, wire)
			}
		})
	}
}

func TestProdex043713WebSocketIdleLiveRolloutRequiresTerminalAndSameSession(t *testing.T) {
	for _, tc := range []struct {
		name    string
		records []map[string]any
		want    string
	}{
		{"accepted terminal", []map[string]any{{"type": "event_msg", "payload": map[string]any{"type": "task_complete", "error": map[string]any{"codex_error_info": "other", "message": prodex043713WebSocketIdleMessage}}}}, "transport"},
		{"intermediate retry", []map[string]any{{"type": "event_msg", "payload": map[string]any{"type": "error", "will_retry": true, "codex_error_info": "other", "message": prodex043713WebSocketIdleMessage}}}, ""},
		{"foreign completion", []map[string]any{{"type": "task_complete", "session_id": "00000000-0000-4000-8000-000000000002", "error": map[string]any{"codex_error_info": "other", "message": prodex043713WebSocketIdleMessage}}}, ""},
		{"next activity overrides", []map[string]any{{"type": "task_complete", "error": map[string]any{"codex_error_info": "other", "message": prodex043713WebSocketIdleMessage}}, {"type": "event_msg", "payload": map[string]any{"type": "agent_message"}}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout-"+recovery04378Session+".jsonl")
			identity, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]any{"id": recovery04378Session}})
			if err := os.WriteFile(path, append(identity, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			checkpoint := captureRecoveryCheckpoint04360(path)
			if !checkpoint.valid {
				t.Fatal("checkpoint invalid")
			}
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			records := append([]map[string]any{{"type": "response_item", "payload": map[string]any{"role": "user"}}}, tc.records...)
			for _, record := range records {
				wire, _ := json.Marshal(record)
				if _, err := file.Write(append(wire, '\n')); err != nil {
					t.Fatal(err)
				}
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			// The live monitor (not the exited-child fallback scanner) drains all
			// records and drops intermediate retries or later activity.
			// The post-child scanner independently tests terminal classification.
			fragment, ok, _ := checkpoint.readAppendedRecovery04360(context.Background(), recovery04378Session, 0)
			if !ok {
				t.Fatal("failed to read rollout")
			}
			got, _, _, _ := scanRecoveryClassSince04377(context.Background(), recovery04378Session, fragment, false)
			if got != tc.want {
				t.Fatalf("class=%q want=%q", got, tc.want)
			}
		})
	}
}
