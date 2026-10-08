package gemini

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestProdex04357GeminiStreamRuntimeProcessesThoughtFunctionAndLaterText(t *testing.T) {
	state := &geminiStreamState{responseID: "resp_gemini_1"}
	var output bytes.Buffer
	err := state.consume(&output, []byte(`{
		"candidates":[{"content":{"parts":[
			{"text":"ignored","thought":true,"functionCall":{"id":"call_1","args":{"a":1,"z":"🌍"}}},
			{"text":"later"}
		]}},{"content":{"parts":[{"text":"also later"}]}}]
	}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "event: response.function_call_arguments.delta") ||
		!strings.Contains(text, `"call_id":"call_1"`) ||
		!strings.Contains(text, `"delta":"{\"a\":1,\"z\":\"🌍\"}"`) {
		t.Fatalf("function-call stream event = %s", text)
	}
	for _, want := range []string{"response.reasoning_summary_text.delta", "ignored", "response.output_text.delta", "later"} {
		if !strings.Contains(text, want) {
			t.Fatalf("runtime Gemini chunk missing %q: %s", want, text)
		}
	}
}

func TestProdex04361GeminiStreamPreservesThoughtSignature(t *testing.T) {
	state := &geminiStreamState{responseID: "resp_gemini_signature"}
	var output bytes.Buffer
	if err := state.consume(&output, []byte(`{
		"candidates":[{"content":{"parts":[{"functionCall":{
			"id":"call_sig","name":"shell","args":{"cmd":"pwd"},"thoughtSignature":"sig-1"
		}}]}}]
	}`), nil); err != nil {
		t.Fatal(err)
	}
	if err := state.complete(&output, nil); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "event: response.function_call_arguments.delta") ||
		!strings.Contains(text, `"thought_signature":"sig-1"`) {
		t.Fatalf("signature delta event = %s", text)
	}
	if !strings.Contains(text, `"gemini_thought_signature":"sig-1"`) {
		t.Fatalf("signature completion item = %s", text)
	}
}

func TestProdex04357GeminiStreamRuntimeFallbackCallIDAndUnicodeThoughtTyping(t *testing.T) {
	state := &geminiStreamState{responseID: "resp_gemini_2"}
	var sparse bytes.Buffer
	if err := state.consume(&sparse, []byte(`{
		"candidates":[{"content":{"parts":[{"text":"ignored","functionCall":{"id":7}}]}}]
	}`), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sparse.String(), `"delta":"{}"`) ||
		!strings.Contains(sparse.String(), `"call_id":"call_gemini_`) {
		t.Fatalf("sparse runtime function-call event = %s", sparse.String())
	}

	state = &geminiStreamState{responseID: "resp_gemini_3"}
	var unicode bytes.Buffer
	if err := state.consume(&unicode, []byte(`{
		"candidates":[{"content":{"parts":[{"text":"héllo 🌍","thought":"true"}]}}]
	}`), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unicode.String(), "event: response.output_text.delta") ||
		!strings.Contains(unicode.String(), "héllo 🌍") ||
		strings.Contains(unicode.String(), "reasoning_summary") {
		t.Fatalf("Unicode non-boolean thought event = %s", unicode.String())
	}
}

func TestProdex04357GeminiStreamMetadataPreservesPresenceAndWhitespace(t *testing.T) {
	state := &geminiStreamState{responseID: "resp_gemini_1", model: "existing-model"}
	root := map[string]any{
		"responseId": nil, "id": "ignored",
		"modelVersion": json.Number("7"), "model": "ignored",
		"usageMetadata": nil,
		"candidates": []any{
			map[string]any{"finishReason": " "},
			map[string]any{"finishReason": "STOP"},
		},
	}
	encoded, _ := json.Marshal(root)
	var output bytes.Buffer
	if err := state.consume(&output, encoded, nil); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("Unicode-whitespace finish reason emitted terminal output: %s", output.String())
	}
	if state.responseID != "resp_gemini_1" || state.model != "existing-model" || state.finish != "" {
		t.Fatalf("stream metadata identity/model/finish = %q / %q / %q", state.responseID, state.model, state.finish)
	}
	if state.usage == nil ||
		state.usage["input_tokens"] != uint64(0) ||
		state.usage["output_tokens"] != uint64(0) ||
		state.usage["total_tokens"] != uint64(0) {
		t.Fatalf("null usage metadata = %#v", state.usage)
	}
	if state.metadata["finishReason"] != " " {
		t.Fatalf("raw whitespace finish metadata = %#v", state.metadata)
	}

	state = &geminiStreamState{responseID: "resp_gemini_1", model: "existing-model"}
	root = map[string]any{
		"id": "", "model": "",
		"candidates": []any{map[string]any{"finishReason": " STOP "}},
	}
	encoded, _ = json.Marshal(root)
	output.Reset()
	if err := state.consume(&output, encoded, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "event: response.completed") {
		t.Fatalf("nonblank raw finish reason did not complete: %s", output.String())
	}
	if state.responseID != "" || state.model != "" || state.finish != " STOP " {
		t.Fatalf("explicit-empty identity/model or raw finish lost = %q / %q / %q", state.responseID, state.model, state.finish)
	}
}

func TestProdex04357GeminiStreamRuntimeCompletesOnDoneSentinel(t *testing.T) {
	state := &geminiStreamState{responseID: "resp_gemini_4"}
	var output bytes.Buffer
	err := state.consume(&output, []byte("[DONE]"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "event: response.completed") || !state.completed {
		t.Fatalf("[DONE] did not complete runtime stream: output=%q completed=%t", output.String(), state.completed)
	}
}
