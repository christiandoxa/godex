//go:build linux && amd64

package codex

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func (provider *codex0163LoopbackProvider) handleAsyncCompaction(
	writer http.ResponseWriter, flusher http.Flusher, callNumber int,
) bool {
	if callNumber != 2 {
		return false
	}
	item := map[string]any{
		"type": "compaction", "encrypted_content": "ASYNC_COMPACTION_SUMMARY",
	}
	writeCodex0163SSE(writer, flusher, "response.created", map[string]any{
		"type": "response.created", "response": map[string]any{"id": "resp-async-compact"},
	})
	writeCodex0163SSE(writer, flusher, "response.output_item.done", map[string]any{
		"type": "response.output_item.done", "item": item,
	})
	writeCodex0163SSE(writer, flusher, "response.completed", map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": "resp-async-compact", "status": "completed",
			"output": []any{item}, "usage": map[string]any{
				"input_tokens": 200, "output_tokens": 0, "total_tokens": 200,
			},
		},
	})
	_, _ = writer.Write([]byte("data: [DONE]\n\n"))
	return true
}

func TestProdex04380OfficialCodex0163LoopbackAsyncCompactionLifecycle(t *testing.T) {
	app, provider := newCodex0163AppServer(t, codex0163LoopbackAsyncCompaction)
	initializeCodex0163AppServer(t, app)
	threadID := startCodex0163Thread(t, app)
	app.beginOperation()
	if _, err := app.request(3, "turn/start", map[string]any{
		"threadId": threadID,
		"input":    []any{map[string]any{"type": "text", "text": "async seed"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.waitIdle(); err != nil {
		t.Fatal(err)
	}
	callContext, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var calls []codex0163LoopbackCall
	for len(calls) < 2 {
		call, err := provider.waitCall(callContext)
		if err != nil {
			t.Fatal(err)
		}
		calls = append(calls, call)
	}
	compactCall := calls[1]
	if compactCall.path != "/v1/responses" {
		t.Fatalf("background remote compaction path = %q, want /v1/responses", compactCall.path)
	}
	var request map[string]any
	if err := json.Unmarshal(compactCall.body, &request); err != nil {
		t.Fatalf("decode background remote compaction request: %v", err)
	}
	input, ok := request["input"].([]any)
	if !ok {
		t.Fatalf("background remote compaction input = %#v, want array", request["input"])
	}
	triggerIndex := -1
	for index, raw := range input {
		item, ok := raw.(map[string]any)
		if ok && item["type"] == "compaction_trigger" {
			if triggerIndex >= 0 || len(item) != 1 {
				t.Fatalf("background remote compaction trigger item = %#v, want one terminal type-only item", item)
			}
			triggerIndex = index
		}
	}
	if triggerIndex != len(input)-1 {
		t.Fatalf("background remote compaction trigger index = %d, want %d", triggerIndex, len(input)-1)
	}
	app.beginOperation()
	if _, err := app.request(4, "turn/start", map[string]any{
		"threadId": threadID,
		"input":    []any{map[string]any{"type": "text", "text": "async followup"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.waitIdle(); err != nil {
		t.Fatal(err)
	}
	followup, err := provider.waitCall(callContext)
	if err != nil {
		t.Fatal(err)
	}
	var followupRequest map[string]any
	if err := json.Unmarshal(followup.body, &followupRequest); err != nil {
		t.Fatalf("decode follow-up request: %v", err)
	}
	followupInput, ok := followupRequest["input"].([]any)
	if !ok {
		t.Fatalf("follow-up input = %#v, want array", followupRequest["input"])
	}
	adopted := false
	for _, raw := range followupInput {
		item, ok := raw.(map[string]any)
		if ok && item["type"] == "compaction" && item["encrypted_content"] == "ASYNC_COMPACTION_SUMMARY" {
			adopted = true
			break
		}
	}
	if !adopted {
		t.Fatalf("follow-up request omitted adopted background compaction: %s", followup.body)
	}
	provider.assertNoCall(t, 250*time.Millisecond)
}
