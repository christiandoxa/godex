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

func TestProdex04384GeminiChatBuiltinDeclarationsPreservePipeline(t *testing.T) {
	const function = `{"function":{"description":"synthetic tool","name":"synthetic_lookup","parameters":{"type":"object"}},"type":"function"}`
	const media = `{"type":"code_execution","function":null,"extra":[{"nested":{"type":"input_image","path":null}}]}`
	fixtures := []struct {
		name, tools, wireTools, message string
		status                          int
	}{
		{name: "key-presence", tools: `{"codeExecution":null}`},
		{name: "key-non-string-type", tools: `{"type":7,"description":null,"strict":null,"urlContext":false}`},
		{name: "key-unknown-type", tools: `{"type":"unsupported","computerUse":{}}`, status: 502},
		{name: "function-null", tools: `{"type":"code_execution","function":null}`, status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"},
		{name: "function-name", tools: `{"type":"computerUse","function":{"parameters":{}}}`, status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0].function.name` must be a non-empty string"},
		{name: "function-parameters", tools: `{"type":"web_search","function":{"name":"synthetic_lookup"}}`, status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required"},
		{name: "function-valid", tools: `{"type":"code_execution","function":{"name":"ignored","parameters":{},"strict":true}}`},
		{name: "description-null", tools: `{"type":"code_execution","description":null}`, status: 502},
		{name: "strict-string", tools: `{"type":"code_execution","strict":"false"}`, status: 502},
		{name: "strict-true", tools: `{"type":"code_execution","strict":true}`},
		{name: "function-key", tools: `{"type":"function","name":"synthetic_lookup","codeExecution":{}}`, status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required"},
		{name: "mixed-deduplication", tools: `{"type":"codeExecution"},` + function + `,{"type":"urlContext"},` + function, wireTools: function},
		{name: "count-after-projection", tools: strings.Repeat(`{"type":"code_execution"},`, 129) + function, wireTools: function},
		{name: "mcp-name", tools: `{"type":"code_execution","name":"mcp__fs__lookup","description":"synthetic tool","parameters":{"type":"object"}}`, wireTools: strings.Replace(function, "synthetic_lookup", "mcp__fs__lookup", 1)},
		{name: "web-mcp-name", tools: `{"type":"web_search","name":"mcp__fs__lookup","parameters":{}}`},
		{name: "web-options", tools: `{"type":"web_search","search_context_size":"high","allowed_domains":["synthetic.invalid"]}`},
		{name: "recursive-media", tools: media},
		{name: "media-file-path", tools: strings.ReplaceAll(media, `"path":null`, `"file_path":false`)},
		{name: "media-camel-path", tools: strings.ReplaceAll(media, `"path":null`, `"filePath":{}`)},
		{name: "media-without-path", tools: strings.ReplaceAll(media, `"path":null`, `"url":"synthetic.invalid"`), status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"},
		{name: "path-without-media", tools: strings.ReplaceAll(media, "input_image", "message"), status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"},
		{name: "media-later", tools: `{"type":"code_execution","function":null},` + media, status: 400, message: "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"},
		{name: "media-earlier", tools: media + `,{"type":"function","name":"synthetic_lookup"}`, status: 400, message: "invalid_tool_declaration: Gemini request field `tools[1].function.parameters` is required"},
		{name: "media-host-validation", tools: strings.TrimSuffix(media, "}") + `,"description":null}`, status: 502},
	}
	for _, kind := range []string{"web_search", "web_search_preview", "web_search_preview_2099", "web_fetch", "url_context", "urlContext", "web_fetch_preview", "web_fetch_preview_2099", "code_interpreter", "code_execution", "codeExecution", "computer", "computer_use", "computerUse", "computer_use_preview", "computer_custom"} {
		fixtures = append(fixtures, struct {
			name, tools, wireTools, message string
			status                          int
		}{name: kind, tools: `{"type":"` + kind + `"}`})
	}
	for _, fixture := range fixtures {
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
						t.Fatalf("builtin declaration lost validation/no-dispatch: error=%v attempts=%d", err, len(requests))
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
					t.Fatal("builtin declaration lost attempt count, status or response")
				}
				want := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + stream
				if stream == "true" {
					want += `,"stream_options":{"include_usage":true}`
				}
				if fixture.wireTools != "" {
					want += `,"tools":[` + fixture.wireTools + `]`
				}
				if got := <-requests; got != want+"}" {
					t.Fatalf("builtin Chat wire = %s, want %s}", got, want)
				}
			})
		}
	}
}
