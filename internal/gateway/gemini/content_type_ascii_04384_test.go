package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Prodex 0.438.4 local_rewrite_response_dispatch does
// `content_type.to_ascii_lowercase().contains("text/event-stream")`.
// Go Unicode strings.ToLower mistakenly accepts the long-s U+017F as ASCII
// 's', turning a JSON buffer into an invented SSE stream.
func TestProdex04384GeminiContentTypeClassificationASCIIOnly(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		isSSE     bool
	}{
		{"canonical", "text/event-stream", true},
		{"ascii-upper", "TEXT/EVENT-STREAM; CHARSET=UTF-8", true},
		{"mixed-ASCII", "Text/Event-Stream", true},
		{"long-s-confusable", "text/event-ſtream", false},
		{"long-s-uppercase", "TEXT/EVENT-ſTREAM", false},
		{"application-json", "application/json", false},
		{"noncanonical-text", "text/event-source", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := geminiContentTypeIsSSE04384(tc.raw); got != tc.isSSE {
				t.Fatalf("Gemini Content-Type=%q SSE=%t want=%t", tc.raw, got, tc.isSSE)
			}
		})
	}
}

func TestProdex04384GeminiUnicodeSSELookalikeStaysBufferedJSON(t *testing.T) {
	for _, chatCompatible := range []bool{false, true} {
		name := "native"
		if chatCompatible {
			name = "chat-compatible"
		}
		t.Run(name, func(t *testing.T) {
			upstream := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview"}`))}
			upstream.Header.Set("Content-Type", "text/event-ſtream")
			var responseErr error
			var body []byte
			if chatCompatible {
				translated, err := translateGeminiChatOrNativeResponse043711(upstream, nil)
				responseErr = err
				if translated != nil {
					defer translated.Body.Close()
					body, _ = io.ReadAll(translated.Body)
					if translated.Header.Get("Content-Type") != "application/json; charset=utf-8" {
						t.Fatalf("SSE lookalike rewrote MIME: %v", translated.Header)
					}
				}
			} else {
				translated, err := translateResponse(upstream, nil)
				responseErr = err
				if translated != nil {
					defer translated.Body.Close()
					body, _ = io.ReadAll(translated.Body)
					if strings.Contains(translated.Header.Get("Content-Type"), "event-stream") {
						t.Fatalf("SSE lookalike classified as stream: %v", translated.Header)
					}
				}
			}
			if responseErr != nil {
				t.Fatal(responseErr)
			}
			var parsed map[string]any
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatalf("Gemini buffered JSON lost: %s err=%v", body, err)
			}
			if parsed["object"] != "response" {
				t.Fatalf("buffered Gemini response missing: %+v", parsed)
			}
			outputArray, _ := parsed["output"].([]any)
			if len(outputArray) == 0 {
				t.Fatalf("buffered Gemini output empty: %+v", parsed)
			}
		})
	}
}
