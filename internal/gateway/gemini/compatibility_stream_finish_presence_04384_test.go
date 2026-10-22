package gemini

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiChatSSEPreservesFinishReasonPresence(t *testing.T) {
	for _, fixture := range []struct{ name, first, second, want string }{
		{"empty replaces stop", `"stop"`, `""`, `""`},
		{"initial empty survives null", `""`, `null`, `""`},
		{"null retains prior", `"stop"`, `null`, `"stop"`},
		{"wrong type retains prior", `"stop"`, `false`, `"stop"`},
		{"absent", ``, ``, ``},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			var wire bytes.Buffer
			for _, value := range []string{fixture.first, fixture.second} {
				fields := `"delta":{"content":"synthetic"}`
				if value != "" {
					fields += `,"finish_reason":` + value
				}
				if err := state.accept(&wire, []byte(`{"choices":[{`+fields+`}]}`)); err != nil {
					t.Fatal(err)
				}
				if state.done || strings.Contains(wire.String(), "response.completed") {
					t.Fatal("finish reason committed the stream before DONE")
				}
			}
			if err := state.accept(&wire, []byte("[DONE]")); err != nil {
				t.Fatal(err)
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			var finish string
			for _, frame := range frames {
				var event struct {
					Type     string `json:"type"`
					Response struct {
						Metadata struct {
							Gemini struct {
								FinishReason json.RawMessage `json:"finish_reason"`
							}
						} `json:"metadata"`
					} `json:"response"`
				}
				if json.Unmarshal(frame, &event) == nil && event.Type == "response.completed" {
					finish = string(event.Response.Metadata.Gemini.FinishReason)
				}
			}
			if finish != fixture.want || strings.Count(wire.String(), "event: response.completed") != 1 {
				t.Fatalf("finish reason=%s, want %s", finish, fixture.want)
			}
		})
	}
}
