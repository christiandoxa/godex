package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiChatCallSignaturesSurviveProjection(t *testing.T) {
	for _, fixture := range []struct{ name, fields, extra, legacy, duplicate, kind string }{
		{name: "absent"},
		{name: "canonical", fields: `,"gemini_thought_signature":"synthetic-signature"`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`},
		{name: "snake", fields: `,"thought_signature":"synthetic-signature"`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`},
		{name: "camel", fields: `,"thoughtSignature":"synthetic-signature"`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`},
		{name: "provider", fields: `,"provider_specific_fields":{"thought_signature":"synthetic-signature"}`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`},
		{name: "extra", fields: `,"extra_content":{"google":{"thought_signature":"synthetic-signature","ignored":true},"ignored":true}`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`},
		{name: "priority", fields: `,"gemini_thought_signature":"synthetic-signature","extra_content":{"google":{"thought_signature":"ignored"}}`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`},
		{name: "skip-empty-and-nonstring", fields: `,"gemini_thought_signature":null,"thought_signature":"","thoughtSignature":7,"provider_specific_fields":{"thought_signature":"synthetic-signature"}`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`},
		{name: "blank-is-barrier", fields: `,"gemini_thought_signature":" \t\u2003","thought_signature":"ignored"`, legacy: `,"gemini_thought_signature":" \t "`},
		{name: "control-space-is-barrier", fields: `,"gemini_thought_signature":"\u001c","thought_signature":"ignored"`, legacy: `,"gemini_thought_signature":"\u001c"`},
		{name: "nested-ignored", fields: `,"function":{"thought_signature":"ignored"}`},
		{name: "duplicate-first-wins", fields: `,"thought_signature":"synthetic-signature"`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`, duplicate: `,{"type":"function_call","call_id":"synthetic-call","name":"ignored","arguments":"{}","thought_signature":"ignored"}`},
		{name: "duplicate-absent-is-barrier", duplicate: `,{"type":"function_call","call_id":"synthetic-call","name":"ignored","arguments":"{}","thought_signature":"ignored"}`},
		{name: "custom", kind: "custom_tool_call", fields: `,"thought_signature":"synthetic-signature"`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`},
		{name: "mcp", kind: "mcp_call", fields: `,"thought_signature":"synthetic-signature"`, extra: `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			requests := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				requests <- string(body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]}}]}`)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			kind, arguments, wireArguments := fixture.kind, `"arguments":"{}"`, `{}`
			if kind == "" {
				kind = "function_call"
			}
			if kind == "custom_tool_call" {
				arguments, wireArguments = `"input":"synthetic-input"`, `{\"input\":\"synthetic-input\"}`
			}
			body := `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"type":"` + kind + `","call_id":"synthetic-call","name":"synthetic_lookup",` + arguments + fixture.fields + `}` + fixture.duplicate + `,{"type":"function_call_output","call_id":"synthetic-call","output":"synthetic-result"}]}`
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			want := `{"messages":[{"content":"same request","role":"user"},{"content":"","role":"assistant","tool_calls":[{` + fixture.extra + `"function":{"arguments":"` + wireArguments + `","name":"synthetic_lookup"}` + fixture.legacy + `,"id":"synthetic-call","type":"function"}]},{"content":"synthetic-result","role":"tool","tool_call_id":"synthetic-call"}],"model":"gemini-3.1-pro-preview","stream":false}`
			if response.StatusCode != 200 || len(requests) != 1 {
				t.Fatalf("signature request status=%d attempts=%d", response.StatusCode, len(requests))
			}
			if got := <-requests; got != want {
				t.Fatalf("signature wire differs: got=%s want=%s", got, want)
			}
		})
	}
}
