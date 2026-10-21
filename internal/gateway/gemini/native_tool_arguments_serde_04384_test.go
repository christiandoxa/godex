package gemini

import (
	"encoding/json"
	"testing"
)

// Tagged Gemini response_tool_calls.rs builds function_call arguments with
// serde_json::to_string(&args_value), before applying any shell RTK policy.
// A non-shell function must preserve canonical serde strings and numeric
// spellings rather than Go JSON's HTML/JS escapes or raw json.Number text.
func TestProdex04384GeminiNativeFunctionArgumentsUseSerdeCanonicalJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		args any
		want string
	}{
		{"canonical-empty", map[string]any{}, `{}`},
		{"HTML-preserved", map[string]any{"html": "<tag>&"}, `{"html":"<tag>&"}`},
		{"unicode-line-preserved", map[string]any{"unicode": "\u2028\u2029"}, "{\"unicode\":\"\u2028\u2029\"}"},
		{"decimal-normalized", map[string]any{"a": json.Number("1.0000000"), "b": json.Number("1e-5")}, `{"a":1.0,"b":0.00001}`},
		{"large-f64-scientific", map[string]any{"large": json.Number("1e20")}, `{"large":1e+20}`},
		{"nested-ordered", map[string]any{"z": []any{map[string]any{"b": "<", "a": json.Number("1e-7")}}, "a": true}, `{"a":true,"z":[{"a":1e-7,"b":"<"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := geminiNativeToolCallItem(map[string]any{}, map[string]any{"name": "read", "id": "call-1", "args": tc.args}, "fallback")
			if item["arguments"] != tc.want {
				t.Fatalf("Gemini canonical arguments %#v yielded %q want %q", tc.args, item["arguments"], tc.want)
			}
		})
	}
}

func TestProdex04384GeminiNativeFunctionArgumentsSurviveResponsesEnvelope(t *testing.T) {
	part := map[string]any{"functionCall": map[string]any{
		"name": "read", "id": "call-synthetic", "args": map[string]any{"decimal": json.Number("1e-5"), "text": "<script>"},
	}}
	root := map[string]any{"responseId": "resp-canonical-args", "modelVersion": "gemini-3.1-pro-preview", "candidates": []any{
		map[string]any{"finishReason": "STOP", "content": map[string]any{"role": "model", "parts": []any{part}}},
	}}
	response := geminiNativeResponsesValue(root, nil, 1)
	output, _ := response["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("Gemini function call absent: %#v", response)
	}
	call, _ := output[0].(map[string]any)
	want := `{"decimal":0.00001,"text":"<script>"}`
	if call["arguments"] != want {
		t.Fatalf("Gemini function call arguments=%q want=%q", call["arguments"], want)
	}
}

func TestProdex04384GeminiRTKWrappedArgumentsKeepSerdeCanonicalFields(t *testing.T) {
	args := map[string]any{
		"cmd": "git status", "annotation": "<tag>&", "separator": "\u2028", "fraction": json.Number("1e-5"),
	}
	item := geminiNativeToolCallItem(map[string]any{}, map[string]any{"name": "exec_command", "args": args}, "call-1")
	want := "{\"annotation\":\"<tag>&\",\"cmd\":\"rtk git status\",\"fraction\":0.00001,\"separator\":\"\u2028\"}"
	if item["arguments"] != want {
		t.Fatalf("Gemini RTK wrapped canonical arguments=%q want=%q", item["arguments"], want)
	}
}

// Gemini's published response_tool_calls/rtk.rs only applies RTK when the
// ORIGINAL flat function name is exactly "shell" or "exec_command". Go's
// chatcompat RTK wrapper also supports "exec" and the Gemini adapter used
// a shortened namespace member, previously wrapping unrelated tool calls.
func TestProdex04384GeminiNativeRTKMatchesExactFlatToolName(t *testing.T) {
	for _, tc := range []struct{ name, wantCommand string }{
		{"shell", "rtk git status"},
		{"exec_command", "rtk git status"},
		{"exec", "git status"},
		{"mcp__tools__shell", "git status"},
		{"mcp__tools__exec_command", "git status"},
		{"mcp__tools__exec", "git status"},
		{"namespace.shell", "git status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := geminiNativeToolCallItem(map[string]any{}, map[string]any{
				"name": tc.name, "args": map[string]any{"cmd": "git status"}}, "call-1")
			got, _ := item["arguments"].(string)
			want := `{"cmd":"` + tc.wantCommand + `"}`
			if got != want {
				t.Fatalf("Gemini tool %q RTK arguments=%q want=%q item=%#v", tc.name, got, want, item)
			}
		})
	}
}
