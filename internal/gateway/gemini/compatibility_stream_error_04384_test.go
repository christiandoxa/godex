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

func TestProdex04384GeminiAPIKeyEmbeddedErrorPrecedesCreatedAndContent(t *testing.T) {
	for _, tc := range []struct{ name, raw, code, message string }{
		{"string", `"boom"`, "provider_stream_error", "boom"},
		{"null", `null`, "provider_stream_error", "Provider stream returned an embedded error"},
		{"primary-null", `{"type":null,"code":"SECONDARY","message":null,"detail":"must-not-borrow"}`, "provider_stream_error", "Provider stream returned an embedded error"},
		{"empty", `{"type":"","message":""}`, "", ""},
		{"signed-max", `{"code":9223372036854775807}`, "9223372036854775807", "Provider stream returned an embedded error"},
		{"integral-float", `{"code":1.0}`, "provider_stream_error", "Provider stream returned an embedded error"},
		{"detail", `{"code":" E ","detail":"详情 🚀"}`, " E ", "详情 🚀"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunk := `{"error":` + tc.raw + `,"choices":[{"delta":{"content":"must-not-emit"}}]}`
			wire := "data: " + chunk + "\n\ndata: " + `{"choices":[{"delta":{"content":"must-not-emit-later"}}]}` + "\n\ndata: [DONE]\n\n"
			response := translateGeminiChatCompatibleStream043711(&http.Response{StatusCode: http.StatusOK,
				Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil)
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(data), decoder.Finish()...)
			if len(frames) != 1 || !bytes.HasPrefix(data, []byte("event: response.failed\r\n")) {
				t.Fatalf("embedded error leaked creation/content/completion: %s", data)
			}
			var event map[string]any
			if err := json.Unmarshal(frames[0], &event); err != nil {
				t.Fatal(err)
			}
			failure := event["response"].(map[string]any)["error"].(map[string]any)
			if failure["code"] != tc.code || failure["message"] != tc.message || event["created_at"] == nil || event["sequence_number"] != float64(0) {
				t.Errorf("API-key error code/message/sequence mismatch: %#v", event)
			}
		})
	}
}

func TestProdex04384GeminiAPIKeyEmbeddedErrorRedactsAndKeepsCommittedIdentity(t *testing.T) {
	bearer := strings.Join([]string{"fixture", "04384", "bearer"}, "_")
	state := &geminiChatCompatibleStream043711{id: "resp-owned", created: 1}
	var output bytes.Buffer
	if err := state.accept(&output, []byte(`{"choices":[{"delta":{"content":"committed"}}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := state.accept(&output, []byte(`{"id":"rejected-id","error":{"code":"synthetic","message":"Authorization: Bearer `+bearer+`"}}`)); err != nil {
		t.Fatal(err)
	}
	wire := output.String()
	if !state.done || strings.Count(wire, "event: response.failed") != 1 || strings.Contains(wire, bearer) || !strings.Contains(wire, "Authorization: Bearer <redacted>") || strings.Contains(wire, "rejected-id") || strings.Contains(wire, "event: response.completed") {
		t.Error("embedded provider error lost terminal ownership or redaction")
	}
}
