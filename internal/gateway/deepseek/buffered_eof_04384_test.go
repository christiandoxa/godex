package deepseek

import (
	"errors"
	"strings"
	"testing"
)

func TestProdex04384BufferedKernelAcceptedArgumentsKeepOuterCause(t *testing.T) {
	for _, arguments := range []string{`1e309`, `"\uD800"`, strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128)} {
		cause := errors.New("kernel output JSON is invalid")
		err := deepSeekToolArgumentError("tool_search", arguments, cause)
		if !errors.Is(err, cause) {
			t.Fatalf("accepted kernel argument replaced the outer cause: %v", err)
		}
	}
}

func TestProdex04384BufferedContainerEOFUsesFirstSerdeDiagnostic(t *testing.T) {
	for _, fixture := range []struct{ arguments, diagnostic string }{
		{`[`, "EOF while parsing a list at line 1 column 1"},
		{`[1`, "EOF while parsing a list at line 1 column 2"},
		{`[1,`, "EOF while parsing a value at line 1 column 3"},
		{`{`, "EOF while parsing an object at line 1 column 1"},
		{`{"k"`, "EOF while parsing an object at line 1 column 4"},
		{`{"k":`, "EOF while parsing a value at line 1 column 5"},
		{`{"k":1,`, "EOF while parsing a value at line 1 column 7"},
		{"\n[1,  \n", "EOF while parsing a value at line 3 column 0"},
		{`{"v":"\uD800","k":`, "unexpected end of hex escape at line 1 column 13"},
		{`{"v":1e309,"k":`, "number out of range at line 1 column 10"},
	} {
		for _, name := range []string{"lookup", "tool_search", "apply_patch"} {
			t.Run(name+"/"+fixture.arguments, func(t *testing.T) {
				_, err := deepSeekResponseToolItem(map[string]any{"function": map[string]any{
					"name": name, "arguments": fixture.arguments,
				}})
				want := "DeepSeek returned malformed JSON arguments for tool call `" + name + "`: " + fixture.diagnostic
				if err == nil || err.Error() != want {
					t.Fatalf("EOF diagnostic = %v, want %q", err, want)
				}
			})
		}
	}
}
