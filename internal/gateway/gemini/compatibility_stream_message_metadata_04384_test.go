package gemini

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiChatSSEAccumulatesMessageMetadata(t *testing.T) {
	for _, fixture := range []struct{ name, first, last, refusal, annotations string }{
		{"refusal", `"refusal":"synthetic-"`, `"refusal":"refusal<>&/🌍\n"`, `"synthetic-refusal<>&/🌍\n"`, ""},
		{"annotations", `"annotations":[{"large":9007199254740993},null]`, `"annotations":[18446744073709551615,null,[],false]`, "", `[{"large":9007199254740993},null,18446744073709551615,null,[],false]`},
		{"combined", `"refusal":"one","annotations":[1]`, `"refusal":"two","annotations":[2]`, `"onetwo"`, `[1,2]`},
		{"null retains", `"refusal":"one","annotations":[1]`, `"refusal":null,"annotations":null`, `"one"`, `[1]`},
		{"wrong types retain", `"refusal":"one","annotations":[1]`, `"refusal":false,"annotations":{}`, `"one"`, `[1]`},
		{"empty", `"refusal":"","annotations":[]`, `"refusal":"","annotations":[]`, "", ""},
		{"wrong types ignored", `"refusal":{},"annotations":false`, `"refusal":123,"annotations":"ignored"`, "", ""},
		{"absent", "", "", "", ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			var wire bytes.Buffer
			for _, fields := range []string{fixture.first, fixture.last} {
				packet := `{"choices":[{"delta":{` + fields + `},"finish_reason":"stop"},{"delta":{"refusal":"ignored","annotations":["ignored"]}}]}`
				if err := state.accept(&wire, []byte(packet)); err != nil {
					t.Fatal(err)
				}
				if state.done {
					t.Fatal("message metadata committed before DONE")
				}
			}
			for range 2 {
				if err := state.accept(&wire, []byte("[DONE]")); err != nil {
					t.Fatal(err)
				}
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			var completed struct {
				Response struct {
					Output   []any
					Metadata struct {
						Gemini struct{ Refusal, Annotations json.RawMessage }
					}
				}
			}
			if len(frames) != 2 || json.Unmarshal(frames[1], &completed) != nil || len(completed.Response.Output) != 0 || strings.Count(wire.String(), "event: response.completed") != 1 {
				t.Fatal("message-only metadata changed stream lifecycle or fabricated output")
			}
			metadata := completed.Response.Metadata.Gemini
			if string(metadata.Refusal) != fixture.refusal || string(metadata.Annotations) != fixture.annotations {
				t.Fatalf("refusal=%s annotations=%s, want %s %s", metadata.Refusal, metadata.Annotations, fixture.refusal, fixture.annotations)
			}
		})
	}
}

func TestGeminiChatSSEMessageMetadataLimitFailsOnceWithoutCompletion(t *testing.T) {
	for _, fixture := range []struct {
		name, fields string
		bytes        int
	}{
		{"refusal", `"refusal":"abc"`, 3},
		{"annotations", `"annotations":[1]`, 3},
		{"combined", `"refusal":"a","annotations":[1]`, 4},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{
				id: "resp_deepseek_synthetic", created: 1,
				messageMetadataBytes: geminiStreamMetadataMaxBytes04384 - fixture.bytes,
			}
			packet := []byte(`{"choices":[{"delta":{` + fixture.fields + `}}]}`)
			var wire bytes.Buffer
			if err := state.accept(&wire, packet); err != nil || state.done || state.messageMetadataBytes != geminiStreamMetadataMaxBytes04384 {
				t.Fatalf("metadata at limit failed: %v", err)
			}
			refusal, annotations := state.refusal.String(), len(state.annotations)
			if err := state.accept(&wire, packet); err != nil {
				t.Fatal(err)
			}
			if err := state.accept(&wire, []byte("[DONE]")); err != nil {
				t.Fatal(err)
			}
			if !state.done || state.refusal.String() != refusal || len(state.annotations) != annotations || state.messageMetadataBytes != geminiStreamMetadataMaxBytes04384 {
				t.Fatal("overflow retained metadata or left the stream open")
			}
			if strings.Count(wire.String(), "event: response.failed") != 1 || strings.Contains(wire.String(), "response.completed") || strings.Contains(wire.String(), "response.output_item") {
				t.Fatal("metadata overflow fabricated output or completed the stream")
			}
		})
	}
}
