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

func TestProdex04384GeminiChatToolKindsPreservePipeline(t *testing.T) {
	const nested = `"function":{"name":"ignored_nested","parameters":{}}`
	const alias = `"name":"mcp__fs__lookup","parameters":{"type":"object"}`
	const projected = `{"function":{"name":"mcp__fs__lookup","parameters":{"type":"object"}},"type":"function"}`
	const unsupported = "invalid_tool_declaration: Gemini request field `tools[0]` is not a supported tool declaration"
	for _, fixture := range []struct {
		name, tools, wireTools, message string
		status                          int
	}{
		{name: "missing", tools: `{}`, status: 400, message: unsupported},
		{name: "null", tools: `{"type":null}`, status: 400, message: unsupported},
		{name: "number", tools: `{"type":7}`, status: 400, message: unsupported},
		{name: "array", tools: `{"type":[]}`, status: 400, message: unsupported},
		{name: "unknown", tools: `{"type":"unsupported"}`, status: 400, message: unsupported},
		{name: "nested", tools: `{` + nested + `}`},
		{name: "nested-null-type", tools: `{"type":null,` + nested + `}`},
		{name: "nested-number-type", tools: `{"type":7,` + nested + `}`},
		{name: "nested-empty-type", tools: `{"type":"",` + nested + `}`, status: 502},
		{name: "nested-unknown-type", tools: `{"type":"unsupported",` + nested + `}`, status: 502},
		{name: "function-null", tools: `{"function":null}`, status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"},
		{name: "mcp", tools: `{` + alias + `}`, wireTools: projected},
		{name: "mcp-alias", tools: `{"type":null,"name":"mcp__fs__lookup","parametersJsonSchema":{"type":"object"}}`, wireTools: projected},
		{name: "mcp-metadata", tools: `{"description":null,"strict":null,` + alias + `}`, wireTools: projected},
		{name: "mcp-strict", tools: `{"strict":true,` + alias + `}`, status: 502},
		{name: "mcp-null-schema", tools: `{"name":"mcp__fs__lookup","parameters":null}`, status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0]` translates to an invalid declaration: invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"},
		{name: "mcp-invalid-name", tools: `{"name":"mcp__fs.bad","parameters":{}}`, status: 502},
		{name: "mcp-unknown-type", tools: `{"type":"unsupported",` + alias + `}`, status: 502},
		{name: "mcp-nested", tools: `{` + nested + `,` + alias + `}`, wireTools: projected},
		{name: "media", tools: `{"extra":[{"type":"input_image","path":null}]}`},
		{name: "mixed", tools: `{` + nested + `},{"type":"function",` + alias + `}`, wireTools: projected},
		{name: "mcp-empty-segment", tools: `{"name":"mcp__","parameters":{"type":"object"}}`, wireTools: strings.ReplaceAll(projected, "mcp__fs__lookup", "mcp__")},
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
				body := `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + stream + `,"tools":[` + fixture.tools + `]}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if fixture.status != 0 {
					var failure *proxymodel.Error
					if response != nil || !errors.As(err, &failure) || failure.StatusCode != fixture.status || len(requests) != 0 {
						t.Fatalf("tool kind lost validation/no-dispatch: error=%v attempts=%d", err, len(requests))
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
					t.Fatal("tool kind lost attempt count, status or response")
				}
				want := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + stream
				if stream == "true" {
					want += `,"stream_options":{"include_usage":true}`
				}
				if fixture.wireTools != "" {
					want += `,"tools":[` + fixture.wireTools + `]`
				}
				if got := <-requests; got != want+"}" {
					t.Fatalf("tool-kind Chat wire = %s, want %s}", got, want)
				}
			})
		}
	}
}
