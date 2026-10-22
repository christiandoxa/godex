package deepseek

import "testing"

func TestProdex04384BufferedStringEOFKeepsFirstSerdeDiagnostic(t *testing.T) {
	for _, fixture := range []struct{ arguments, diagnostic string }{
		{`"abc`, "EOF while parsing a string at line 1 column 4"},
		{`"abc\`, "EOF while parsing a string at line 1 column 5"},
		{`{"k":"abc`, "EOF while parsing a string at line 1 column 9"},
		{`{"ke`, "EOF while parsing a string at line 1 column 4"},
		{`["abc`, "EOF while parsing a string at line 1 column 5"},
		{"\n\"雪", "EOF while parsing a string at line 2 column 4"},
		{`"\uD8`, "EOF while parsing a string at line 1 column 5"},
		{`"\uD800`, "EOF while parsing a string at line 1 column 7"},
		{`"\uD800\`, "EOF while parsing a string at line 1 column 8"},
		{`"\uD800\"`, "unexpected end of hex escape at line 1 column 9"},
		{`"\uD800x`, "unexpected end of hex escape at line 1 column 8"},
		{`"\uDC00`, "lone leading surrogate in hex escape at line 1 column 7"},
		{`"\uD800\uDC00`, "EOF while parsing a string at line 1 column 13"},
		{`"\uD800\u0041`, "lone leading surrogate in hex escape at line 1 column 13"},
		{`{"a":tru`, "EOF while parsing a value at line 1 column 8"},
		{`{"v":1e309,"s":"abc`, "number out of range at line 1 column 10"},
		{`{"v":"\uD800","s":"abc`, "unexpected end of hex escape at line 1 column 13"},
	} {
		for _, name := range []string{"lookup", "tool_search", "apply_patch"} {
			t.Run(name+"/"+fixture.arguments, func(t *testing.T) {
				_, err := deepSeekResponseToolItem(map[string]any{"function": map[string]any{
					"name": name, "arguments": fixture.arguments,
				}})
				want := "DeepSeek returned malformed JSON arguments for tool call `" + name + "`: " + fixture.diagnostic
				if err == nil || err.Error() != want {
					t.Fatalf("string EOF diagnostic = %v, want %q", err, want)
				}
			})
		}
	}
}

func TestProdex04384BufferedSearchSurrogateEscapeUsesOuterPosition(t *testing.T) {
	_, err := deepSeekResponseToolItem(map[string]any{"id": "call-differential",
		"function": map[string]any{"name": "tool_search", "arguments": `"\uD800\""`}})
	want := "DeepSeek Mojo returned invalid response tool-call JSON: unexpected end of hex escape at line 1 column 99"
	if err == nil || err.Error() != want {
		t.Fatalf("outer surrogate escape diagnostic = %v, want %q", err, want)
	}
}
