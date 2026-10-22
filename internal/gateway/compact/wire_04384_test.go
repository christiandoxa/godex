package compact

import (
	"io"
	"net/http"
	"reflect"
	"testing"
)

func TestProdex04384LocalCompactWireContract(t *testing.T) {
	const body = `{"model":"deepseek-v4-pro","input":[{"role":"user","content":[{"type":"input_text","text":"same request"}]}]}`
	want := `{"output":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` + SummaryPrefix + `\n\nLocal Prodex compact fallback summary.\n\nModel: deepseek-v4-pro\nOriginal input items: 1\nRetained recent items: 1\n\nRecent conversation and tool state:\n- item: same request\n  user"}]}]}`
	for _, provider := range []string{"deepseek", "anthropic", "gemini", "kiro"} {
		t.Run(provider, func(t *testing.T) {
			response, err := LocalFallback([]byte(body), provider, "local-policy")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			got, err := io.ReadAll(response.Body)
			if err != nil || string(got) != want {
				t.Fatalf("compact wire differs: body=%s err=%v", got, err)
			}
			headers := http.Header{"Content-Type": {"application/json; charset=utf-8"},
				"X-Prodex-Compact-Mode": {"local-fallback"}, "X-Prodex-Compact-Provider": {provider},
				"X-Prodex-Compact-Degraded": {"true"}, "X-Prodex-Compact-Reason": {"local-policy"}}
			if response.StatusCode != 200 || !reflect.DeepEqual(response.Header, headers) {
				t.Fatalf("compact status/headers differ: %d %v", response.StatusCode, response.Header)
			}
		})
	}
}

func TestProdex04384SemanticCompactUsesOrderedUnescapedUTF8Envelope(t *testing.T) {
	response, err := Semantic(" \t<>&\u2028\u2029\n\"\\\tend\n ", "gemini")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	want := `{"output":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` + SummaryPrefix + `\n\n<>&` + "\u2028\u2029" + `\n\"\\\tend"}]}]}`
	if err != nil || string(got) != want {
		t.Fatalf("semantic compact escaped bytes or reordered fields: %s err=%v", got, err)
	}
	wantHeaders := http.Header{"Content-Type": {"application/json; charset=utf-8"},
		"X-Prodex-Compact-Mode": {"semantic"}, "X-Prodex-Compact-Provider": {"gemini"}}
	if !reflect.DeepEqual(response.Header, wantHeaders) {
		t.Fatalf("semantic compact headers differ: %v", response.Header)
	}
}
