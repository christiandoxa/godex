package deepseek

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The exact Prodex 0.436.1 binary emits these five events for one text chunk.
// In particular it does not append response.output_text.done and it keeps
// created_at/object out of the nested completed response.
func TestProdex04361DeepSeekStreamExactEventSequence(t *testing.T) {
	state := newDeepSeekChatStreamState(123, nil, nil, deepSeekConversationStore{})
	state.createdAt = time.Unix(1, 0)
	var wire strings.Builder
	for _, value := range []string{
		`{"id":"chatcmpl-differential-stream","model":"deepseek-v4-pro","created":1,"choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-differential-stream","model":"deepseek-v4-pro","created":1,"choices":[{"delta":{"content":"synthetic-ok"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-differential-stream","model":"deepseek-v4-pro","created":1,"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		"[DONE]",
	} {
		event, emitted, err := state.observe([]byte(value))
		if err != nil {
			t.Fatal(err)
		}
		if emitted {
			wire.Write(event)
		}
	}
	expected := []string{
		"response.created", "response.output_item.added",
		"response.output_text.delta", "response.output_item.done",
		"response.completed",
	}
	var got []string
	var itemIDs []string
	for _, block := range strings.Split(wire.String(), "\r\n\r\n") {
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\r\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "event: ") ||
			!strings.HasPrefix(lines[1], "data: ") {
			t.Fatalf("malformed wire SSE frame: %q", block)
		}
		label := strings.TrimPrefix(lines[0], "event: ")
		got = append(got, label)
		var data map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &data); err != nil {
			t.Fatal(err)
		}
		if data["type"] != label || data["sequence_number"] != float64(len(got)-1) {
			t.Fatalf("SSE event metadata drift: label %s data %#v", label, data)
		}
		if label == "response.output_item.added" || label == "response.output_item.done" {
			item := data["item"].(map[string]any)
			itemIDs = append(itemIDs, item["id"].(string))
		}
		if label == "response.completed" {
			response := data["response"].(map[string]any)
			if _, ok := response["object"]; ok {
				t.Fatal("completed SSE response wrongly includes object")
			}
			if _, ok := response["created_at"]; ok {
				t.Fatal("completed SSE response wrongly includes created_at")
			}
			if response["model"] != "deepseek-v4-pro" {
				t.Fatalf("completed model = %#v", response["model"])
			}
		}
	}
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Fatalf("Prodex SSE event order: got %v, want %v", got, expected)
	}
	if len(itemIDs) != 2 || itemIDs[0] != itemIDs[1] || !strings.HasPrefix(itemIDs[0], "msg_deepseek_") {
		t.Fatalf("SSE item identifier inconsistent: %v", itemIDs)
	}
}

func TestProdex04361DeepSeekSSEHeadersRejectUpstreamFingerprintLeak(t *testing.T) {
	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":         {"text/event-stream"},
			"X-Synthetic-Upstream": {"differential-v1"},
		},
		Body: io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
	}
	result, err := translateResponseWithConversation(upstream, nil, deepSeekConversationStore{}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if value := result.Header.Get("Content-Type"); value != "text/event-stream; charset=utf-8" {
		t.Fatalf("DeepSeek SSE content type = %q", value)
	}
	if value := result.Header.Get("X-Synthetic-Upstream"); value != "" {
		t.Fatalf("upstream transport marker escaped into Codex response: %q", value)
	}
}
