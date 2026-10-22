package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiBufferedWireUsesSerdeJSON(t *testing.T) {
	for _, tc := range []struct {
		name, text, metadata string
		want                 []string
	}{
		{"ASCII-control", "synthetic-ok", `"avgLogprobs":1.0`, []string{`"text":"synthetic-ok"`, `"avgLogprobs":1.0`}},
		{"HTML-and-Unicode", "<tag>&\u2028\u2029", `"avgLogprobs":1.0`, []string{"\"text\":\"<tag>&\u2028\u2029\""}},
		{"canonical-float", "synthetic-ok", `"avgLogprobs":1.000000,"logprobsResult":{"fraction":1e-5,"negativeZero":-0}`, []string{`"avgLogprobs":1.0,`, `"fraction":0.00001,`, `"negativeZero":-0.0`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := `{"responseId":"resp-synthetic","modelVersion":"gemini-3.1-pro-preview","candidates":[{"content":{"parts":[{"text":"` + tc.text + `"}]},"finishReason":"STOP",` + tc.metadata + `}]}`
			for _, chat := range []bool{false, true} {
				upstream := &http.Response{StatusCode: http.StatusOK,
					Header: http.Header{"Content-Type": {"application/json"}, "X-Synthetic": {"preserved"}},
					Body:   io.NopCloser(strings.NewReader(wire))}
				var translated *proxymodel.Response
				var err error
				if chat {
					translated, err = translateGeminiChatOrNativeResponse043711(upstream, nil, nil)
				} else {
					translated, err = translateResponse(upstream, nil)
				}
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(translated.Body)
				translated.Body.Close()
				if err != nil || !json.Valid(body) || translated.Header.Get("X-Synthetic") != "preserved" {
					t.Fatalf("invalid buffered response body=%q err=%v", body, err)
				}
				for _, want := range tc.want {
					if !strings.Contains(string(body), want) {
						t.Errorf("chat=%v canonical wire missing %q: %s", chat, want, body)
					}
				}
			}
		})
	}
}

func TestProdex04384GeminiApplyPatchFallbackUsesSerdeJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		args any
		want string
	}{
		{"empty-control", map[string]any{}, `{}`},
		{"null-control", nil, `null`},
		{"HTML", map[string]any{"markup": "<tag>&"}, `{"markup":"<tag>&"}`},
		{"Unicode", map[string]any{"separator": "\u2028\u2029"}, "{\"separator\":\"\u2028\u2029\"}"},
		{"numeric", map[string]any{"fraction": json.Number("1e-5"), "decimal": json.Number("1.00000")}, `{"decimal":1.0,"fraction":0.00001}`},
		{"array", []any{json.Number("1e20"), "<"}, `[1e+20,"<"]`},
		{"explicit-input", map[string]any{"input": "literal <tag>"}, "literal <tag>"},
		{"content-alias", map[string]any{"content": "literal <tag>"}, "literal <tag>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := geminiNativeApplyPatchInput(tc.args); got != tc.want {
				t.Fatalf("custom patch input=%q want=%q", got, tc.want)
			}
			item := geminiNativeToolCallItem(nil, map[string]any{"name": "apply_patch", "args": tc.args}, "call-synthetic")
			if item["type"] != "custom_tool_call" || item["input"] != tc.want {
				t.Fatalf("buffered custom tool changed input: %#v", item)
			}
		})
	}
}
