package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiAPIKeyCompactUsesRawChatPipeline(t *testing.T) {
	const instructions = "Compact the supplied coding-agent transcript into one durable continuation summary. Preserve the user's goals, repository instructions, decisions, files changed, exact identifiers, commands and test results, unresolved failures, current worktree state, and the next concrete steps. Remove redundant narration and obsolete intermediate reasoning. Do not call tools. Return only the continuation summary, with no preamble or completion claim."
	const prefix = "Another language model started to solve this problem and produced a summary of its thinking process. You also have access to the state of the tools that were used by that language model. Use this to build on the work that has already been done and avoid duplicating work. Here is the summary produced by the other language model, use the information in this summary to assist with your own analysis:"
	for _, fixture := range []struct {
		name, payload, text, reason string
		invalidInput                bool
		limit                       int
	}{
		{name: "native", payload: `{"candidates":[{"content":{"parts":[{"text":"summary"}]}}]}`, text: "summary"},
		{name: "chat", payload: `{"choices":[{"message":{"content":" summary "}},{"message":{"content":"ignored"}}],"output_text":"ignored"}`, text: "summary"},
		{name: "parts", payload: `{"choices":[{"message":{"content":[{"type":"other","text":" first "},"ignored",{"text":" second "}]}}]}`, text: "first\nsecond"},
		{name: "responses", payload: `{"output":[{"type":"message","content":" first "},{"type":"message","content":[{"text":"second"}]}]}`, text: "first\nsecond"},
		{name: "wrapped", payload: `{"response":{"output_text":"inner"},"traceId":""}`, text: "inner"},
		{name: "untraced", payload: `{"response":{"output_text":"inner"}}`, reason: "invalid-response"},
		{name: "thought", payload: `{"candidates":[{"content":{"parts":[{"thought":true,"text":"hidden"},{"thought":false,"text":"visible"}]}},{"content":{"parts":[{"text":"ignored"}]}}]}`, text: "visible"},
		{name: "empty", payload: `{}`, reason: "invalid-response"},
		{name: "invalid-json", payload: `{`, reason: "invalid-response"},
		{name: "invalid-input", invalidInput: true, reason: "invalid-response"},
		{name: "under", limit: (4 << 20) - 1, text: "bounded"},
		{name: "at", limit: 4 << 20, text: "bounded"},
		{name: "over", limit: (4 << 20) + 1, reason: "provider-error"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			requests := make(chan string, 8)
			payload := fixture.payload
			if fixture.limit != 0 {
				payload = `{"output_text":"bounded","padding":"` + strings.Repeat("x", fixture.limit-len(`{"output_text":"bounded","padding":""}`)) + `"}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if r.URL.RequestURI() != "/v1beta/openai/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-key" {
					t.Error("Compact lost the Chat path, dropped query or owning synthetic credential")
				}
				requests <- string(body)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Upstream-Decoy", "must-not-leak")
				_, _ = io.WriteString(w, payload)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			input := `[]`
			if fixture.invalidInput {
				input = `"invalid"`
			}
			body := `{"model":"gemini-3.8-flash","input":` + input + `,"stream":true,"instructions":"ignored","tools":null,"tool_choice":false,"include":null,"text":null}`
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: mountPath + "/responses/compact", RawQuery: "must=drop", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			content, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			mode := "semantic"
			text := "Semantic continuation summary:\n" + fixture.text + "\n\nContinue the active user request. Do not merely acknowledge repository, optimizer, or environment instructions."
			if fixture.reason != "" {
				mode = "local-fallback"
				text = "Local Prodex compact fallback summary.\n\nModel: gemini-3.8-flash\nOriginal input items: 0\nRetained recent items: 0\n\nRecent conversation and tool state:\n- No parseable recent message or tool content was found."
			}
			quoted, _ := json.Marshal(prefix + "\n\n" + text)
			want := `{"output":[{"type":"message","role":"user","content":[{"type":"input_text","text":` + string(quoted) + `}]}]}`
			if string(content) != want || response.StatusCode != 200 || response.Header.Get("Content-Type") != "application/json; charset=utf-8" || response.Header.Get("X-Prodex-Compact-Mode") != mode || response.Header.Get("X-Prodex-Compact-Provider") != "gemini" || response.Header.Get("X-Prodex-Compact-Reason") != fixture.reason || response.Header.Get("X-Upstream-Decoy") != "" {
				t.Fatal("Compact lost exact raw summary, fallback body or headers")
			}
			if fixture.invalidInput {
				if len(requests) != 0 {
					t.Fatal("invalid Compact input reached upstream")
				}
				return
			}
			wire, _ := json.Marshal(map[string]any{"model": "gemini-3.8-flash", "stream": false,
				"messages": []any{map[string]any{"role": "system", "content": instructions}, map[string]any{"role": "user", "content": instructions}}})
			if len(requests) != 1 || <-requests != string(wire) {
				t.Fatal("Compact lost the selected model, rewrite, exact Chat wire or one-attempt contract")
			}
		})
	}
}
