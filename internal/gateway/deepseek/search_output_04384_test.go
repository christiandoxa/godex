package deepseek

import (
	"strings"
	"testing"
)

func TestProdex04384BufferedSearchOutputUsesOuterSerdeDiagnostic(t *testing.T) {
	const prefix = "DeepSeek Mojo returned invalid response tool-call JSON: "
	for _, fixture := range []struct {
		name, arguments, diagnostic string
		id                          any
	}{
		{"overflow", "1e309", "number out of range at line 1 column 95", "call-differential"},
		{"overflow_line", "\n1e309", "number out of range at line 2 column 5", "call-differential"},
		{"high_surrogate", `"\uD800"`, "unexpected end of hex escape at line 1 column 98", "call-differential"},
		{"low_surrogate", `"\uDC00"`, "lone leading surrogate in hex escape at line 1 column 97", "call-differential"},
		{"surrogate_pair_error", `"\uD800\u0041"`, "lone leading surrogate in hex escape at line 1 column 103", "call-differential"},
		{"depth_over", strings.Repeat("[", 127) + "0" + strings.Repeat("]", 127), "recursion limit exceeded at line 1 column 217", "call-differential"},
		{"early_surrogate", `{"v":"\uD800","deep":` + strings.Repeat("[", 200) + "0" + strings.Repeat("]", 200) + `}`, "unexpected end of hex escape at line 1 column 103", "call-differential"},
		{"missing_id", "1e309", "number out of range at line 1 column 84", nil},
		{"nonstring_id", "1e309", "number out of range at line 1 column 84", 7},
		{"empty_id", "1e309", "number out of range at line 1 column 78", ""},
		{"escaped_utf8_id", "1e309", "number out of range at line 1 column 95", "call-\"\n<&\u2028雪"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			call := map[string]any{"function": map[string]any{"name": "tool_search", "arguments": fixture.arguments}}
			if fixture.id != nil {
				call["id"] = fixture.id
			}
			item, err := deepSeekResponseToolItem(call)
			if err == nil || err.Error() != prefix+fixture.diagnostic || item != nil {
				t.Fatalf("structured output diagnostic = %v, want %q", err, prefix+fixture.diagnostic)
			}
		})
	}
}

func TestProdex04384BufferedSearchOutputKeepsBoundaryControls(t *testing.T) {
	for _, arguments := range []string{"null", `"\uD800\uDC00"`, strings.Repeat("[", 126) + "0" + strings.Repeat("]", 126)} {
		item, err := deepSeekResponseToolItem(map[string]any{"id": "call-differential",
			"function": map[string]any{"name": "tool_search", "arguments": arguments}})
		if err != nil || item["type"] != "tool_search_call" || item["call_id"] != "call-differential" {
			t.Fatalf("valid structured output rejected: %v", err)
		}
	}
	arguments := strings.Repeat("[", 257) + "0" + strings.Repeat("]", 257)
	_, err := deepSeekResponseToolItem(map[string]any{"id": "call-differential",
		"function": map[string]any{"name": "tool_search", "arguments": arguments}})
	want := "DeepSeek returned malformed JSON arguments for tool call `tool_search`: recursion limit exceeded at line 1 column 128"
	if err == nil || err.Error() != want {
		t.Fatalf("kernel rejection lost inner diagnostic: %v", err)
	}
}

func TestProdex04384BufferedSearchOutputFailureKeepsEarlierOutput(t *testing.T) {
	response := executeDeepSeekBufferedResponse(t, map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{
			"content": "keep earlier text",
			"tool_calls": []any{
				map[string]any{"id": "call-first", "function": map[string]any{"name": "lookup", "arguments": `{}`}},
				map[string]any{"id": "call-differential", "function": map[string]any{"name": "tool_search", "arguments": "1e309"}},
			},
		}}},
	})
	output, _ := response["output"].([]any)
	err, _ := response["error"].(map[string]any)
	if response["status"] != "failed" || len(output) != 2 || err["code"] != "invalid_tool_call_arguments" ||
		err["message"] != "DeepSeek Mojo returned invalid response tool-call JSON: number out of range at line 1 column 95" {
		t.Fatalf("structured output error lost failed envelope or earlier items: %#v", response)
	}
}
