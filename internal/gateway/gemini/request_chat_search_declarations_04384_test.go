package gemini

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiChatSearchDeclarationsPreservePipeline(t *testing.T) {
	const parameters = `{"additionalProperties":false,"properties":{"query":{"type":"string"}},"required":["query"],"type":"object"}`
	const declaration = `{"function":{"name":"tool_search","parameters":` + parameters + `},"type":"function"}`
	const missing = "invalid_tool_declaration: Gemini request field `tools[0]` translates to an invalid declaration: invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required"
	for _, fixture := range []struct {
		name, tool, wire, message string
		status                    int
	}{
		{name: "basic", tool: `{"type":"tool_search","parameters":` + parameters + `}`, wire: declaration},
		{name: "fixed-name", tool: `{"type":"tool_search","name":"supplied","namespace":"ignored","parameters":` + parameters + `}`, wire: declaration},
		{name: "description", tool: `{"type":"tool_search","description":" synthetic ","parameters":` + parameters + `}`, wire: strings.Replace(declaration, `"name":`, `"description":" synthetic ","name":`, 1)},
		{name: "blank-description", tool: `{"type":"tool_search","description":" \t\u2003","parameters":` + parameters + `}`, wire: declaration},
		{name: "description-null", tool: `{"type":"tool_search","description":null,"parameters":{}}`, status: 502},
		{name: "strict-ignored", tool: `{"type":"tool_search","strict":true,"parameters":` + parameters + `}`, wire: declaration},
		{name: "strict-string", tool: `{"type":"tool_search","strict":"false","parameters":{}}`, status: 502},
		{name: "missing-parameters", tool: `{"type":"tool_search"}`, status: 400, message: missing},
		{name: "alias-ignored", tool: `{"type":"tool_search","parametersJsonSchema":{}}`, status: 400, message: missing},
		{name: "null-parameters", tool: `{"type":"tool_search","parameters":null}`, status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0]` translates to an invalid declaration: invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"},
		{name: "nested-ignored", tool: `{"type":"tool_search","function":{"name":"ignored","parameters":{}},"parameters":` + parameters + `}`, wire: declaration},
		{name: "nested-missing", tool: `{"type":"tool_search","function":{"name":"ignored","parameters":{}}}`, wire: `{"function":{"name":"tool_search"},"type":"function"}`},
		{name: "builtin-key", tool: `{"type":"tool_search","codeExecution":null}`, wire: `{"function":{"name":"tool_search"},"type":"function"}`},
		{name: "media-exemption", tool: `{"type":"tool_search","extra":{"type":"input_image","path":null}}`, wire: `{"function":{"name":"tool_search"},"type":"function"}`},
		{name: "function-media-exemption", tool: `{"type":"function","name":"synthetic_lookup","extra":{"type":"input_image","path":null}}`, wire: `{"function":{"name":"synthetic_lookup"},"type":"function"}`},
		{name: "function-media-null", tool: `{"type":"function","name":"synthetic_lookup","parameters":null,"extra":{"type":"input_image","path":null}}`, status: 502},
		{name: "mcp-and-search", tool: `{"type":"tool_search","name":"mcp__fs__lookup","parameters":` + parameters + `}`, wire: strings.Replace(declaration, "tool_search", "mcp__fs__lookup", 1) + "," + declaration},
		{name: "duplicate", tool: `{"type":"tool_search","parameters":` + parameters + `},{"type":"tool_search","parameters":` + parameters + `}`, wire: declaration},
		{name: "conflict", tool: `{"type":"tool_search","parameters":` + parameters + `},{"type":"tool_search","parameters":{"type":"object","properties":{}}}`, status: 502},
	} {
		for _, stream := range []string{"false", "true"} {
			t.Run(fixture.name+"/stream="+stream, func(t *testing.T) {
				requests := make(chan string, 8)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					requests <- string(body)
					if stream == "true" {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+`{"choices":[{"delta":{"content":"synthetic-ok"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}]}`)
				}))
				defer server.Close()
				pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				body := `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + stream + `,"tools":[` + fixture.tool + `]}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if fixture.status != 0 {
					var failure *proxymodel.Error
					if response != nil || !errors.As(err, &failure) || failure.StatusCode != fixture.status || len(requests) != 0 {
						t.Fatalf("search declaration lost validation/no-dispatch: error=%v attempts=%d", err, len(requests))
					}
					if fixture.message != "" && failure.Message != fixture.message {
						t.Fatalf("validation message = %q, want %q", failure.Message, fixture.message)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				payload, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != 200 || len(requests) != 1 || !strings.Contains(string(payload), "synthetic-ok") {
					t.Fatal("search declaration lost attempt count, status or response")
				}
				want := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + stream
				if stream == "true" {
					want += `,"stream_options":{"include_usage":true}`
				}
				want += `,"tools":[` + fixture.wire + `]}`
				if got := <-requests; got != want {
					t.Fatalf("search Chat wire = %s, want %s", got, want)
				}
			})
		}
	}
}
