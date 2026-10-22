package deepseek

import (
	"strings"
	"testing"
)

func TestProdex04384BufferedOrdinaryArgumentsUseBoundedRawView(t *testing.T) {
	for _, arguments := range []string{
		`truex`, `NaN`, `1e309`, `"\uD800"`, `"\q"`, `[truex]`, `{"v":NaN}`,
		strings.Repeat("[", 256) + "0" + strings.Repeat("]", 256),
		strings.Repeat("[", 257) + strings.Repeat("]", 257),
		"", " \t\n ", `{"cmd":"echo harmless"}`,
	} {
		for _, name := range []string{"lookup", "apply_patch"} {
			item, err := deepSeekResponseToolItem(map[string]any{"id": "call-view", "function": map[string]any{
				"name": name, "arguments": arguments,
			}})
			if err != nil {
				t.Fatalf("raw-view arguments rejected for %s, bytes=%d: %v", name, len(arguments), err)
			}
			field := "arguments"
			if name == "apply_patch" {
				field = "input"
			}
			if item[field] != arguments || item["call_id"] != "call-view" {
				t.Fatalf("opaque arguments or call identity changed: %v", item)
			}
		}
	}
}

func TestProdex04384BufferedRawViewRejectsDepthBeyondReference(t *testing.T) {
	for _, arguments := range []string{
		strings.Repeat("[", 257) + "0" + strings.Repeat("]", 257),
		strings.Repeat("[", 258) + strings.Repeat("]", 258),
	} {
		_, err := deepSeekResponseToolItem(map[string]any{"function": map[string]any{"name": "lookup", "arguments": arguments}})
		want := "DeepSeek returned malformed JSON arguments for tool call `lookup`: recursion limit exceeded at line 1 column 128"
		if err == nil || err.Error() != want {
			t.Fatalf("depth diagnostic = %v, want %q", err, want)
		}
	}
}

func TestProdex04384BufferedKernelFailureKeepsEarlierSerdeDiagnostic(t *testing.T) {
	deep := strings.Repeat("[", 257) + "0" + strings.Repeat("]", 257)
	for _, fixture := range []struct{ arguments, diagnostic string }{
		{`{"v":"\uD800","deep":` + deep + `}`, "unexpected end of hex escape at line 1 column 13"},
		{`{"v":"\uDC00","deep":` + deep + `}`, "lone leading surrogate in hex escape at line 1 column 12"},
		{`{"v":"\uD800\u0041","deep":` + deep + `}`, "lone leading surrogate in hex escape at line 1 column 18"},
		{`{"v":1e309,"deep":` + deep + `}`, "number out of range at line 1 column 10"},
		{"\n" + deep, "recursion limit exceeded at line 2 column 128"},
	} {
		_, err := deepSeekResponseToolItem(map[string]any{"function": map[string]any{"name": "lookup", "arguments": fixture.arguments}})
		want := "DeepSeek returned malformed JSON arguments for tool call `lookup`: " + fixture.diagnostic
		if err == nil || err.Error() != want {
			t.Fatalf("fallback diagnostic = %v, want %q", err, want)
		}
	}
}

func TestProdex04384BufferedStructuredSearchRetainsSerdeAdmission(t *testing.T) {
	for _, arguments := range []string{`truex`, `NaN`, `1e309`, `"\uD800"`, `"\q"`, `[truex]`} {
		if _, err := deepSeekResponseToolItem(map[string]any{"function": map[string]any{
			"name": "tool_search", "arguments": arguments,
		}}); err == nil {
			t.Fatalf("invalid structured search arguments accepted: %q", arguments)
		}
	}
}
