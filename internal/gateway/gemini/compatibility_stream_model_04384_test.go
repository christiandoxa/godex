package gemini

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiChatSSEPreservesModelPresence(t *testing.T) {
	for _, fixture := range []struct{ name, first, second, want string }{
		{"cleared model", `{"model":"first"}`, `{"model":""}`, `""`},
		{"initial empty", `{"model":""}`, `{}`, `""`},
		{"null retains prior", `{"model":"first"}`, `{"model":null}`, `"first"`},
		{"wrong type retains prior", `{"model":"first"}`, `{"model":{}}`, `"first"`},
		{"absent", `{}`, `{}`, ``},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			var wire bytes.Buffer
			for _, packet := range []string{fixture.first, fixture.second, `[DONE]`} {
				if err := state.accept(&wire, []byte(packet)); err != nil {
					t.Fatal(err)
				}
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			var model string
			for _, frame := range frames {
				var event struct {
					Type     string                          `json:"type"`
					Response struct{ Model json.RawMessage } `json:"response"`
				}
				if json.Unmarshal(frame, &event) == nil && event.Type == "response.completed" {
					model = string(event.Response.Model)
				}
			}
			if model != fixture.want {
				t.Fatalf("model=%s, want %s", model, fixture.want)
			}
		})
	}
}
