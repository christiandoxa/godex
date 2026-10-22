package gemini

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiChatSSEReasoningLifecycle(t *testing.T) {
	for _, fixture := range []struct {
		name, first, second, reasoning string
	}{
		{"separate deltas", `{"reasoning_content":"synthetic "}`, `{"reasoning_content":"reason"}`, "synthetic reason"},
		{"mixed text", `{"reasoning_content":"synthetic reason","content":"first "}`, `{}`, "synthetic reason"},
		{"empty control", `{"reasoning_content":""}`, `{}`, ""},
		{"wrong-type control", `{"reasoning_content":12}`, `{}`, ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 111}
			var wire bytes.Buffer
			for _, delta := range []string{fixture.first, fixture.second, `{"content":"last"}`} {
				packet := []byte(`{"model":"gemini-3.1-pro-preview","choices":[{"delta":` + delta + `}]}`)
				if err := state.accept(&wire, packet); err != nil {
					t.Fatal(err)
				}
			}
			if err := state.accept(&wire, []byte("[DONE]")); err != nil {
				t.Fatal(err)
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			var lifecycle []string
			var reasoning string
			for index, frame := range frames {
				var event map[string]any
				if json.Unmarshal(frame, &event) != nil || event["sequence_number"] != float64(index) {
					t.Fatalf("invalid event sequence: %s", frame)
				}
				kind, _ := event["type"].(string)
				if kind == "response.reasoning_summary_text.delta" {
					reasoning += event["delta"].(string)
				}
				if kind == "response.reasoning_summary_part.added" || kind == "response.reasoning_summary_text.done" {
					if event["response_id"] != state.id || event["output_index"] != float64(0) || event["summary_index"] != float64(0) {
						t.Fatalf("reasoning event lost context: %s", frame)
					}
					lifecycle = append(lifecycle, kind)
				}
				if kind == "response.completed" {
					completed := event["response"].(map[string]any)
					metadata, _ := completed["metadata"].(map[string]any)
					provider, _ := metadata["gemini"].(map[string]any)
					if fixture.reasoning != "" && provider["reasoning_content"] != fixture.reasoning {
						t.Fatalf("completed reasoning missing: %s", frame)
					}
				}
			}
			var want []string
			if fixture.reasoning != "" {
				want = []string{"response.reasoning_summary_part.added", "response.reasoning_summary_text.done"}
			}
			if reasoning != fixture.reasoning || !reflect.DeepEqual(lifecycle, want) {
				t.Fatalf("reasoning=%q lifecycle=%v, want %q %v", reasoning, lifecycle, fixture.reasoning, want)
			}
		})
	}
}

func TestProdex04384GeminiReasoningRetentionIsBounded(t *testing.T) {
	state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 111}
	state.reasoning.WriteString(strings.Repeat("a", geminiStreamReasoningMaxBytes04384))
	var wire bytes.Buffer
	if err := state.accept(&wire, []byte(`{"choices":[{"delta":{"reasoning_content":"b","content":"must-not-emit"}}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := state.accept(&wire, []byte("[DONE]")); err != nil {
		t.Fatal(err)
	}
	if !state.done || state.reasoning.Len() != geminiStreamReasoningMaxBytes04384 || strings.Count(wire.String(), "event: response.failed") != 1 || strings.Contains(wire.String(), "must-not-emit") || strings.Contains(wire.String(), "response.completed") {
		t.Fatal("reasoning retention overflow did not terminate once before later content")
	}
}
