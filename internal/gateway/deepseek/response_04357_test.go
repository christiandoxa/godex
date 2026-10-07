package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04357DeepSeekLargeResponsePreservesMetadata(t *testing.T) {
	arguments := `{"payload":"` + strings.Repeat("x", 9<<20) + `"}`
	upstream, err := json.Marshal(map[string]any{
		"id":                 "chatcmpl_large",
		"model":              "deepseek-v4-pro",
		"system_fingerprint": "fp-large",
		"choices": []any{map[string]any{
			"finish_reason": "tool_calls",
			"logprobs":      map[string]any{"content": []any{}},
			"message": map[string]any{
				"content":           "",
				"reasoning_content": "preserve-large-metadata",
				"annotations":       []any{map[string]any{"type": "url_citation", "url": "https://example.test"}},
				"tool_calls": []any{map[string]any{
					"id": "call_large", "type": "function",
					"function": map[string]any{"name": "lookup", "arguments": arguments},
				}},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(upstream) <= 8<<20 || len(upstream) >= 20<<20 {
		t.Fatalf("fixture size = %d, want >8MiB and <20MiB", len(upstream))
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(upstream)
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses", Body: []byte(`{"input":"hello"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("translated large response JSON: %v", err)
	}
	metadata, _ := value["metadata"].(map[string]any)
	provider, _ := metadata["deepseek"].(map[string]any)
	if provider["reasoning_content"] != "preserve-large-metadata" ||
		provider["finish_reason"] != "tool_calls" || provider["system_fingerprint"] != "fp-large" {
		t.Fatalf("large response metadata = %#v", provider)
	}
	output, _ := value["output"].([]any)
	found := false
	for _, raw := range output {
		item, _ := raw.(map[string]any)
		if item["type"] == "function_call" && item["call_id"] == "call_large" {
			found = item["arguments"] == arguments
		}
	}
	if !found {
		t.Fatal("large DeepSeek tool arguments were not preserved")
	}
}
