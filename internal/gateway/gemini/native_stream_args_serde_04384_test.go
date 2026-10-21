package gemini

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Tagged 0.438.4 gemini_provider_core_stream_function_call_delta runs over
// a serde_json::Value and copies the *canonical* args slice into an SSE
// function-call delta and the eventual response.output function_call item.
// Go encoding/json instead HTML/JS-escapes and retains decimal spellings.
func TestProdex04384GeminiStreamCanonicalToolArguments(t *testing.T) {
	for _, tc := range []struct{ name, argument, want string }{
		{"html", `{"html":"<&>"}`, `{"html":"<&>"}`},
		{"unicode-line-separators", `{"separator":"\u2028\u2029"}`, "{\"separator\":\"\u2028\u2029\"}"},
		{"scientific", `{"small":1e-5,"unit":1.0000000}`, `{"small":0.00001,"unit":1.0}`},
		{"nested", `{"z":{"b":1e-7,"a":"<"},"a":"&"}`, `{"a":"&","z":{"a":"<","b":1e-7}}`},
		{"null-present", `null`, `null`},
		{"integer", `7`, `7`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &geminiStreamState{responseID: "resp-synthetic"}
			upstream := []byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read_file","id":"call-synthetic","args":` + tc.argument + `}}]}}]}`)
			var output bytes.Buffer
			if err := state.consume(&output, upstream, nil); err != nil {
				t.Fatal(err)
			}
			if len(state.toolCalls) != 1 || state.toolCalls[0]["arguments"] != tc.want {
				t.Fatalf("Gemini stream args=%s gave toolCalls=%#v want=%q", tc.argument, state.toolCalls, tc.want)
			}
			var parsedDelta string
			for _, frame := range strings.Split(output.String(), "\n\n") {
				for _, line := range strings.Split(frame, "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event map[string]any
					if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
						t.Fatal(err)
					}
					if event["type"] == "response.function_call_arguments.delta" {
						parsedDelta, _ = event["delta"].(string)
					}
				}
			}
			if parsedDelta != tc.want {
				t.Fatalf("Gemini SSE arguments delta=%q want=%q raw=%s", parsedDelta, tc.want, output.String())
			}
			if err := state.complete(&output, nil); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), `"response.completed"`) {
				t.Fatal("stream completion lost after tool args")
			}
		})
	}
}

func TestProdex04384GeminiStreamAbsentToolArgsDefaultOnlyWhenMissing(t *testing.T) {
	for _, tc := range []struct{ name, tail, want string }{
		{"missing", "", `{}`},
		{"explicit-null", `,"args":null`, `null`},
		{"empty-object", `,"args":{}`, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &geminiStreamState{responseID: "resp-test"}
			payload := []byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read_file","id":"call-one"` + tc.tail + `}}]}}]}`)
			if err := state.consume(new(bytes.Buffer), payload, nil); err != nil {
				t.Fatal(err)
			}
			if len(state.toolCalls) != 1 || state.toolCalls[0]["arguments"] != tc.want {
				t.Fatalf("explicit args=%q yielded %#v want=%q", tc.tail, state.toolCalls, tc.want)
			}
		})
	}
}

// The translated SSE JSON itself must preserve the tagged serde JSON string
// encoding, not merely round-trip to the same decoded value. HTML and U+2028
// have observably different wire bytes under encoding/json.Marshal.
func TestProdex04384GeminiSSEEventWireUsesSerdeStringEscaping(t *testing.T) {
	state := &geminiStreamState{responseID: "resp-sse-bytes"}
	var output bytes.Buffer
	payload := []byte(`{"candidates":[{"content":{"parts":[{"text":"<&>\u2028 世界"}]}}]}`)
	if err := state.consume(&output, payload, nil); err != nil {
		t.Fatal(err)
	}
	wire := output.String()
	if !strings.Contains(wire, "<&>\u2028 世界") || strings.Contains(wire, `\u003c`) || strings.Contains(wire, `\u2028`) {
		t.Fatalf("SSE event body not canonical serde UTF-8: %q", wire)
	}
}

func TestProdex04384GeminiStreamNullArgsSurviveCompletedOutput(t *testing.T) {
	state := &geminiStreamState{responseID: "resp-null-args"}
	var output bytes.Buffer
	payload := []byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read_file","id":"call-null","args":null}}]},"finishReason":"STOP"}]}`)
	if err := state.consume(&output, payload, nil); err != nil {
		t.Fatal(err)
	}
	if !state.completed {
		t.Fatal("finish reason did not complete")
	}
	for _, eventFrame := range strings.Split(output.String(), "\n\n") {
		for _, line := range strings.Split(eventFrame, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			if event["type"] != "response.completed" {
				continue
			}
			response := event["response"].(map[string]any)
			items := response["output"].([]any)
			if len(items) != 1 || items[0].(map[string]any)["arguments"] != "null" {
				t.Fatalf("buffered response after streamed null arguments=%#v", items)
			}
			return
		}
	}
	t.Fatal("missing completed output")
}
