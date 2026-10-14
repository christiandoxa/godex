package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Exact 0.437.11 binary trace for a valid OpenAI Chat Completion SSE on the
// Gemini API-key Super route. The existing native-SSE fixture separately
// checks that a native Gemini stream does not invent text deltas.
func TestProdex043711GeminiAPIKeyChatSSECanonicalLifecycleAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"id":"chat-fixture","model":"gemini-3.1-pro-preview","choices":[{"delta":{"content":"synthetic-"},"finish_reason":null}]}`,
			`{"id":"chat-fixture","model":"gemini-3.1-pro-preview","choices":[{"delta":{"content":"ok"},"finish_reason":null}]}`,
			`{"id":"chat-fixture","model":"gemini-3.1-pro-preview","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		}
		for _, event := range events {
			_, _ = io.WriteString(writer, "data: "+event+"\n\n")
		}
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}))
	defer server.Close()
	pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{
		{ID: "key-a", Secret: "synthetic-credential"},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	response, err := pool.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses",
		Body: []byte(`{"model":"gemini-3.1-pro-preview","input":"synthetic","stream":true}`),
	}, proxymodel.Account{ID: "key-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoder := sse.NewDecoder(1 << 20)
	events := append(decoder.Feed(data), decoder.Finish()...)
	var types []string
	var id, itemID string
	for index, raw := range events {
		var event map[string]any
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		kind, _ := event["type"].(string)
		types = append(types, kind)
		if event["sequence_number"] != float64(index) {
			t.Fatalf("noncanonical sequence %d: %+v", index, event)
		}
		switch kind {
		case "response.created":
			created, _ := event["response"].(map[string]any)
			id, _ = created["id"].(string)
			if !strings.HasPrefix(id, "resp_deepseek_") || event["created_at"] == nil {
				t.Fatalf("missing canonical Gemini stream response created: %+v", event)
			}
		case "response.output_item.added":
			item, _ := event["item"].(map[string]any)
			itemID, _ = item["id"].(string)
			if !strings.HasPrefix(itemID, "msg_gemini_") || event["response_id"] != id {
				t.Fatalf("Gemini output_item.added identity mismatch: %+v", event)
			}
			if contents, ok := item["content"].([]any); !ok || len(contents) != 0 {
				t.Fatalf("output_item.added already contained text: %+v", item)
			}
		case "response.output_text.delta":
			if event["response_id"] != id || event["created_at"] == nil {
				t.Fatalf("Gemini text delta lost response context: %+v", event)
			}
		case "response.output_item.done":
			item, _ := event["item"].(map[string]any)
			if item["id"] != itemID || event["response_id"] != id {
				t.Fatalf("Gemini output_item.done identity mismatch: %+v", event)
			}
			content, _ := item["content"].([]any)
			if len(content) != 1 || content[0].(map[string]any)["text"] != "synthetic-ok" {
				t.Fatalf("final Gemini item content incorrect: %+v", item)
			}
		case "response.completed":
			completed, _ := event["response"].(map[string]any)
			output, _ := completed["output"].([]any)
			usage, _ := completed["usage"].(map[string]any)
			meta, _ := completed["metadata"].(map[string]any)
			provider, _ := meta["gemini"].(map[string]any)
			if completed["id"] != id || completed["model"] != "gemini-3.1-pro-preview" ||
				len(output) != 1 || usage["input_tokens"] != float64(2) ||
				usage["output_tokens"] != float64(1) || usage["total_tokens"] != float64(3) ||
				provider["finish_reason"] != "stop" {
				t.Fatalf("Gemini completed lost canonical metadata/usage: %+v", completed)
			}
		}
	}
	want := []string{
		"response.created", "response.output_item.added",
		"response.output_text.delta", "response.output_text.delta",
		"response.output_item.done", "response.completed",
	}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("tagged Gemini Chat SSE lifecycle got %v, want %v; wire=%s", types, want, data)
	}
}
