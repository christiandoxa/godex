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

func TestProdex04384GeminiChatMCPDeclarationsPreserveCompletePipeline(t *testing.T) {
	function := func(name, description, parameters string) string {
		fields := `"name":"` + name + `","parameters":` + parameters
		if description != "" {
			fields = `"description":"` + description + `",` + fields
		}
		return `{"function":{` + fields + `},"type":"function"}`
	}
	const generic = `{"additionalProperties":true,"type":"object"}`
	read := function("mcp__fs__read", "MCP tool read from fs.", generic)
	for _, fixture := range []struct {
		name, tool, wire string
		status           int
	}{
		{name: "basic", tool: `{"type":"mcp","server_label":"fs","allowed_tools":["read"]}`, wire: read},
		{name: "set", tool: `{"type":"mcp_toolset","server_label":"fs","allowed_tools":["read"]}`, wire: read},
		{name: "configs", tool: `{"type":"mcp","server_label":"fs","configs":{"zeta":{},"alpha":{"enabled":false},"beta":{"enabled":true}}}`, wire: function("mcp__fs__beta", "MCP tool beta from fs.", generic) + "," + function("mcp__fs__zeta", "MCP tool zeta from fs.", generic)},
		{name: "disabled-default", tool: `{"type":"mcp","server_label":"fs","default_config":{"enabled":false},"configs":{"zeta":{},"alpha":{"enabled":true}}}`, wire: function("mcp__fs__alpha", "MCP tool alpha from fs.", generic)},
		{name: "union", tool: `{"type":"mcp","server_label":"fs","allowed_tools":[" zeta ","alpha","alpha"],"configs":{"beta":{},"zeta":{}}}`, wire: function("mcp__fs__alpha", "MCP tool alpha from fs.", generic) + "," + function("mcp__fs__beta", "MCP tool beta from fs.", generic) + "," + function("mcp__fs__zeta", "MCP tool zeta from fs.", generic)},
		{name: "sanitize", tool: `{"type":"mcp","server_label":" α git-tools β ","allowed_tools":[" read file "]}`, wire: function("mcp__git_tools__read_file", "MCP tool read file from  α git-tools β .", generic)},
		{name: "prefixed", tool: `{"type":"mcp","server_label":"fs","allowed_tools":["mcp__else__read"]}`, wire: function("mcp__else__read", "MCP tool mcp__else__read from fs.", generic)},
		{name: "alias-type-fallback", tool: `{"type":"mcp","mcp_server_name":null,"server_label":"fs","allowed_tools":["read"]}`, wire: read},
		{name: "description", tool: `{"type":"mcp","server_label":"fs","allowed_tools":["read"],"description":" synthetic description "}`, wire: function("mcp__fs__read", " synthetic description ", generic)},
		{name: "set-strict-not-forwarded", tool: `{"type":"mcp","server_label":"fs","allowed_tools":["read"],"strict":true}`, wire: read},
		{name: "single", tool: `{"type":"mcp_tool","name":"mcp__fs__read","parameters":{"type":"object"}}`, wire: function("mcp__fs__read", "", `{"type":"object"}`)},
		{name: "schema-alias", tool: `{"type":"mcp_tool","name":"mcp__fs__read","parametersJsonSchema":{"type":"object"}}`, wire: function("mcp__fs__read", "", `{"type":"object"}`)},
		{name: "empty", tool: `{"type":"mcp","server_label":"fs","allowed_tools":[]}`, status: 400},
		{name: "no-declarations", tool: `{"type":"mcp","server_label":"fs"}`, status: 400},
		{name: "missing-server", tool: `{"type":"mcp","allowed_tools":["read"]}`, status: 400},
		{name: "empty-server-shadow", tool: `{"type":"mcp","mcp_server_name":"","server_label":"fs","allowed_tools":["read"]}`, status: 400},
		{name: "no-name-segment", tool: `{"type":"mcp","server_label":"fs","allowed_tools":["___"]}`, status: 400},
		{name: "no-server-segment", tool: `{"type":"mcp","server_label":"___","allowed_tools":["read"]}`, status: 400},
		{name: "single-missing-schema", tool: `{"type":"mcp_tool","name":"mcp__fs__read"}`, status: 400},
		{name: "single-primary-null", tool: `{"type":"mcp_tool","name":"mcp__fs__read","parameters":null,"schema":{"type":"object"}}`, status: 400},
		{name: "description-null", tool: `{"type":"mcp","server_label":"fs","allowed_tools":["read"],"description":null}`, status: 502},
		{name: "strict-wrong-type", tool: `{"type":"mcp","server_label":"fs","allowed_tools":["read"],"strict":"true"}`, status: 502},
		{name: "single-strict-rejected", tool: `{"type":"mcp_tool","name":"mcp__fs__read","parameters":{"type":"object"},"strict":true}`, status: 502},
	} {
		for _, stream := range []bool{false, true} {
			label, streamValue := "native", "false"
			if stream {
				label, streamValue = "stream", "true"
			}
			t.Run(fixture.name+"/"+label, func(t *testing.T) {
				requests := make(chan string, 8)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					requests <- string(body)
					if stream {
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
				body := `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + streamValue + `,"tools":[` + fixture.tool + `]}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if fixture.status != 0 {
					var failure *proxymodel.Error
					if response != nil || !errors.As(err, &failure) || failure.StatusCode != fixture.status || len(requests) != 0 {
						t.Fatalf("MCP rejection lost validation stage or no-dispatch: error=%v attempts=%d", err, len(requests))
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
				wire := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + streamValue
				if stream {
					wire += `,"stream_options":{"include_usage":true}`
				}
				wire += `,"tools":[` + fixture.wire + `]}`
				if response.StatusCode != 200 || len(requests) != 1 || <-requests != wire || !strings.Contains(string(payload), "synthetic-ok") {
					t.Fatal("MCP declarations lost exact projection, order, attempts or response")
				}
			})
		}
	}
}

func TestProdex04384GeminiMCPValidationRetainsOriginalIndexAndLaterFailures(t *testing.T) {
	const validMCP = `{"type":"mcp","server_label":"fs","allowed_tools":["read"]}`
	for _, fixture := range []struct{ tools, message string }{
		{`[{"type":"function","name":"valid","parameters":{}},{"type":"mcp","allowed_tools":[]}]`, "invalid_tool_declaration: Gemini request field `tools[1]` is not a supported tool declaration"},
		{`[` + validMCP + `,{"type":"function","name":"invalid","parameters":false}]`, "invalid_tool_declaration: Gemini request field `tools[1].function.parameters` must be an object"},
	} {
		body := []byte(`{"tools":` + fixture.tools + `}`)
		err := validateGeminiChatCompatibleTools04380(body)
		if err == nil || err.Error() != fixture.message {
			t.Fatalf("MCP bridge lost exact original error: got=%v want=%s", err, fixture.message)
		}
	}
}
