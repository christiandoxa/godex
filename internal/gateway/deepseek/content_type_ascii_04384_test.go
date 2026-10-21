package deepseek

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekContentTypeUsesASCIIFoldingOnly(t *testing.T) {
	for _, tc := range []struct {
		name, mime string
		wantSSE    bool
	}{
		{"canonical", "text/event-stream", true},
		{"uppercase", "TEXT/EVENT-STREAM; charset=utf-8", true},
		{"mixedcase", "Text/Event-Stream", true},
		{"long-s-confusable", "text/event-ſtream", false},
		{"long-s-uppercase", "TEXT/EVENT-ſTREAM", false},
		{"buffered-json", "application/json", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if actual := deepSeekContentTypeIsSSE04384(tc.mime); actual != tc.wantSSE {
				t.Fatalf("DeepSeek Content-Type %q is SSE=%t want %t", tc.mime, actual, tc.wantSSE)
			}
		})
	}
}

func TestProdex04384DeepSeekUnicodeSSELookalikeRemainsBufferedJSON(t *testing.T) {
	source := `{"id":"chatcmpl-synthetic","model":"deepseek-v4-pro","choices":[{"message":{"role":"assistant","content":"synthetic-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(source))}
	resp.Header.Set("Content-Type", "text/event-ſtream")
	result, err := translateResponseWithMetadata(resp, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if strings.Contains(result.Header.Get("Content-Type"), "event-stream") {
		t.Fatalf("DeepSeek interpreted Unicode confusable MIME as SSE: %+v", result.Header)
	}
	wire, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(wire, &body); err != nil {
		t.Fatalf("DeepSeek buffered JSON lost: %s err=%v", wire, err)
	}
	if body["object"] != "response" {
		t.Fatalf("DeepSeek buffered response malformed: %#v", body)
	}
	items, ok := body["output"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("DeepSeek buffered response lost assistant output: %#v", body)
	}
}
