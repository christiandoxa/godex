package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiSSEEnforcesProviderWireLimits(t *testing.T) {
	const limit = 4 << 20
	for _, native := range []bool{false, true} {
		packet := `{"choices":[{"delta":{"content":"limit-control"}}]}`
		if native {
			packet = `{"candidates":[{"content":{"parts":[{"text":"limit-control"}]}}]}`
		}
		large := `{"unused":"` + strings.Repeat("a", 2<<20) + `",` + packet[1:]
		for _, tc := range []struct{ name, wire, failure string }{
			{"ordinary-control", "data: " + packet + "\n\ndata: [DONE]\n\n", ""},
			{"larger-than-one-MiB", "data: " + large + "\n\ndata: [DONE]\n\n", ""},
			{"oversized-comment", ":" + strings.Repeat("a", limit) + "\n\ndata: " + packet + "\n\ndata: [DONE]\n\n", "provider SSE line exceeded safe size limit (4194304)"},
			{"oversized-event", "data: \"" + strings.Repeat("a", 2<<20) + "\"\ndata: \"" + strings.Repeat("b", 2<<20) + "\"\n\ndata: [DONE]\n\n", "provider SSE event exceeded safe size limit (4194304)"},
			{"invalid-comment-UTF8", ":" + string([]byte{0xff}) + "\n\ndata: " + packet + "\n\ndata: [DONE]\n\n", "stream did not contain valid UTF-8"},
		} {
			t.Run(tc.name+map[bool]string{true: "/native", false: "/chat"}[native], func(t *testing.T) {
				upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.wire))}
				var body io.ReadCloser
				if native {
					body = translateGeminiNativeStream(upstream, nil).Body
				} else {
					body = translateGeminiChatCompatibleStream043711(upstream, nil, nil).Body
				}
				output, err := io.ReadAll(body)
				body.Close()
				wire := string(output)
				if err != nil {
					t.Fatal(err)
				}
				if tc.failure != "" {
					if strings.Count(wire, "event: response.failed") != 1 || !strings.Contains(wire, `"message":"`+tc.failure+`"`) || strings.Contains(wire, "event: response.completed") || strings.Contains(wire, "limit-control") {
						t.Fatalf("wire limit was skipped or lost failure: %s", wire)
					}
				} else if !strings.Contains(wire, "limit-control") || strings.Count(wire, "event: response.completed") != 1 || strings.Contains(wire, "event: response.failed") {
					t.Fatalf("admissible wire lost content: %s", wire)
				}
			})
		}
	}
}

func TestProdex04384GeminiChatSSEKeepsAdmittedLargeText(t *testing.T) {
	text := strings.Repeat("a", 2<<20) + "large-text-end"
	packet := `{"choices":[{"delta":{"content":"` + text + `"}}]}`
	upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + packet + "\n\ndata: [DONE]\n\n"))}
	response := translateGeminiChatCompatibleStream043711(upstream, nil, nil)
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	decoder := sse.NewDecoder(4 << 20)
	var deltas strings.Builder
	completed := 0
	for _, frame := range append(decoder.Feed(body), decoder.Finish()...) {
		var event map[string]any
		if err := json.Unmarshal(frame, &event); err != nil {
			t.Fatal(err)
		}
		if event["type"] == "response.output_text.delta" {
			deltas.WriteString(event["delta"].(string))
		}
		if event["type"] == "response.completed" {
			completed++
		}
	}
	if deltas.String() != text || completed != 1 || bytes.Contains(body, []byte("event: response.failed")) {
		t.Fatalf("admitted text was dropped during internal frame decode: bytes=%d want=%d completions=%d", deltas.Len(), len(text), completed)
	}
}
