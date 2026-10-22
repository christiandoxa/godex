package gemini

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiChatSSERetainsLatestMetadata(t *testing.T) {
	for _, fixture := range []struct{ name, first, second, fingerprint, laterFingerprint, want string }{
		{"integer and null update", `{"token_id":9007199254740993}`, `null`, `" fp "`, `""`, `{"gemini":{"logprobs":{"token_id":9007199254740993},"system_fingerprint":" fp "}}`},
		{"replacement", `{"old":1}`, `{"new":2}`, `"first"`, `"last"`, `{"gemini":{"logprobs":{"new":2},"system_fingerprint":"last"}}`},
		{"false", `false`, `null`, `12`, `null`, `{"gemini":{"logprobs":false}}`},
		{"zero", `0`, `null`, `null`, `null`, `{"gemini":{"logprobs":0}}`},
		{"floating kind", `1000.0`, `null`, `null`, `null`, `{"gemini":{"logprobs":1000.0}}`},
		{"empty string", `""`, `null`, `null`, `null`, `{"gemini":{"logprobs":""}}`},
		{"empty array", `[]`, `null`, `null`, `null`, `{"gemini":{"logprobs":[]}}`},
		{"empty object", `{}`, `null`, `null`, `null`, `{"gemini":{"logprobs":{}}}`},
		{"null control", `null`, `null`, `null`, `""`, ``},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 111}
			var wire bytes.Buffer
			for _, packet := range []string{
				`{"system_fingerprint":` + fixture.fingerprint + `,"choices":[{"logprobs":` + fixture.first + `,"delta":{"content":"synthetic"}}]}`,
				`{"system_fingerprint":` + fixture.laterFingerprint + `,"choices":[{"logprobs":` + fixture.second + `,"delta":{}}]}`,
				`[DONE]`,
			} {
				if err := state.accept(&wire, []byte(packet)); err != nil {
					t.Fatal(err)
				}
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			var metadata string
			for _, frame := range frames {
				var event struct {
					Type     string                             `json:"type"`
					Response struct{ Metadata json.RawMessage } `json:"response"`
				}
				if json.Unmarshal(frame, &event) == nil && event.Type == "response.completed" {
					metadata = string(event.Response.Metadata)
				}
			}
			if metadata != fixture.want {
				t.Fatalf("metadata=%s, want %s", metadata, fixture.want)
			}
		})
	}
}
