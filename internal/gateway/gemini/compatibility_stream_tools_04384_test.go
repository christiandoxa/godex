package gemini

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
	"github.com/google/uuid"
)

func TestProdex04384GeminiChatToolLifecycleRetainsExactEvents(t *testing.T) {
	for _, fixture := range []struct {
		name, arguments, final, message string
		valid                           bool
	}{
		{"lookup", `{}`, `{}`, "", true},
		{"shell", `{"cmd":"git status"}`, `{"cmd":"rtk git status"}`, "", true},
		{"lookup", `{bad`, "", "Gemini OpenAI-compatible streamed malformed JSON arguments for tool call `lookup` at index 2: invalid JSON", false},
		{"", `{}`, "", "Gemini OpenAI-compatible streamed a tool call without a function name at index 2", false},
	} {
		t.Run(fixture.name+fixture.arguments, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			function := map[string]any{"arguments": fixture.arguments}
			if fixture.name != "" {
				function["name"] = fixture.name
			}
			packet, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 2, "id": "call_synthetic", "function": function}}}}}})
			var wire bytes.Buffer
			for _, raw := range [][]byte{packet, []byte(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`), []byte("[DONE]")} {
				if err := state.accept(&wire, raw); err != nil {
					t.Fatal(err)
				}
			}
			decoder := sse.NewStrictDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			kinds := []string{"response.created", "response.output_item.added", "response.function_call_arguments.delta", "response.failed"}
			if fixture.valid {
				kinds = []string{"response.created", "response.output_item.added", "response.function_call_arguments.delta", "response.output_item.done", "response.completed"}
			}
			if decoder.Err() != nil || len(frames) != len(kinds) {
				t.Fatal("tool lifecycle lost exact event count or framing")
			}
			for index, frame := range frames {
				var event map[string]any
				if json.Unmarshal(frame, &event) != nil || event["type"] != kinds[index] || event["sequence_number"] != float64(index) {
					t.Fatal("tool lifecycle lost event order or sequence")
				}
				if index == 1 {
					item := event["item"].(map[string]any)
					if _, exists := item["arguments"]; exists || event["output_index"] != float64(2) || item["call_id"] != "call_synthetic" {
						t.Fatal("tool announcement lost identity/index or introduced arguments")
					}
				}
				if index == 2 && (event["delta"] != fixture.arguments || event["call_id"] != "call_synthetic" || event["output_index"] != float64(2)) {
					t.Fatal("raw argument delta was transformed or lost its tool index")
				}
				if event["type"] == "response.output_item.done" && event["item"].(map[string]any)["arguments"] != fixture.final {
					t.Fatal("completed tool lost assembled or RTK arguments")
				}
				if event["type"] == "response.failed" {
					failure := event["response"].(map[string]any)["error"].(map[string]any)
					if failure["code"] != "invalid_tool_call_arguments" || failure["message"] != fixture.message {
						t.Fatal("tool validation lost its exact stable diagnostic")
					}
				}
			}
		})
	}
}

func TestProdex04384GeminiChatStreamToolNamesKeepStreamBoundaries(t *testing.T) {
	for _, fixture := range []struct{ name, namespace, short string }{
		{"ns--tool--", "", "ns--tool--"},
		{" ns -- tool ", " ns ", " tool "},
		{"ns.tool", "", "ns.tool"},
		{"mcp____tool", "mcp__", "tool"},
		{"mcp__server__", "", "mcp__server__"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			namespace, short := geminiChatStreamSplitToolName04384(fixture.name)
			if namespace != fixture.namespace || short != fixture.short {
				t.Fatal("stream tool name borrowed an earlier separator or lost original bytes")
			}
		})
	}
}

func TestProdex04384GeminiChatStreamToolFallbackAndExplicitEmptyID(t *testing.T) {
	state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
	var wire bytes.Buffer
	if err := state.accept(&wire, []byte(`{"choices":[{"delta":{"tool_calls":[{"function":{"name":"lookup","arguments":"{}"}}]}}]}`)); err != nil {
		t.Fatal(err)
	}
	fallback := state.toolCalls[0].id
	parsed, err := uuid.Parse(strings.TrimPrefix(fallback, "call_deepseek_"))
	if err != nil || parsed.Version() != 7 || !strings.Contains(wire.String(), fallback) {
		t.Fatal("new tool fallback lost its tagged namespace or UUIDv7")
	}
	for _, packet := range []string{`{"choices":[{"delta":{"tool_calls":[{"id":"","function":{}}]}}]}`, "[DONE]"} {
		if err := state.accept(&wire, []byte(packet)); err != nil {
			t.Fatal(err)
		}
	}
	if state.toolCalls[0].id != "" || !strings.Contains(wire.String(), `"item":{"arguments":"{}","call_id":""`) {
		t.Fatal("explicit empty tool ID borrowed the prior fallback")
	}
}

func TestProdex04384GeminiChatStreamBoundsCompletionText(t *testing.T) {
	state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1, started: true}
	state.text.WriteString(strings.Repeat("x", geminiStreamMetadataMaxBytes04384))
	var wire bytes.Buffer
	if err := state.accept(&wire, []byte(`{"choices":[{"delta":{"content":"must-not-emit"}}]}`)); err != nil {
		t.Fatal(err)
	}
	if !state.done || !state.eof || state.text.Len() != geminiStreamMetadataMaxBytes04384 || strings.Count(wire.String(), "event: response.failed") != 1 || strings.Contains(wire.String(), "must-not-emit") || !strings.Contains(wire.String(), "provider text exceeded safe size limit") {
		t.Fatal("completion text exceeded its budget or leaked the rejected delta")
	}
}
