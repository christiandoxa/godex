package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiMCPEmptyNameRetainsInlineResult(t *testing.T) {
	for _, fixture := range []struct {
		name, result, want string
		earlier, valid     bool
	}{
		{name: "no-result"},
		{name: "orphan", result: `,"output":{"value":1}`},
		{name: "earlier-object", result: `,"output":{"b":1.00,"a":"<","large":9007199254740993}`, want: `{"a":"<","b":1.0,"large":9007199254740993}`, earlier: true},
		{name: "earlier-array", result: `,"output":[{"text":"part"},null]`, want: `[{"text":"part"},null]`, earlier: true},
		{name: "earlier-null", result: `,"output":null`, want: "null", earlier: true},
		{name: "earlier-false", result: `,"output":false,"content":"ignored"`, want: "false", earlier: true},
		{name: "earlier-no-result", earlier: true},
		{name: "earlier-content", result: `,"content":{"value":1}`, want: `{"value":1}`, earlier: true},
		{name: "earlier-result", result: `,"result":[null,false]`, want: `[null,false]`, earlier: true},
		{name: "earlier-error", result: `,"error":false`, want: "false", earlier: true},
		{name: "valid-inline-control", result: `,"output":{"value":1}`, want: `{"value":1}`, valid: true},
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
			first := ""
			if fixture.earlier {
				first = `{"type":"function_call","call_id":"synthetic-call","name":"synthetic_lookup","arguments":"{}"},`
			}
			name := `"name":null,"tool_name":"synthetic_lookup"`
			if fixture.valid {
				name = `"name":"synthetic_lookup"`
			}
			body := `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},` + first + `{"type":"mcp_call","call_id":"synthetic-call",` + name + `,"arguments":"{}"` + fixture.result + `}]}`
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			messages := `{"content":"same request","role":"user"}`
			if fixture.earlier && fixture.result != "" || fixture.valid {
				messages += `,{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"{}","name":"synthetic_lookup"},"id":"synthetic-call","type":"function"}]},{"content":` + fixture.want + `,"role":"tool","tool_call_id":"synthetic-call"}`
			}
			if !fixture.valid {
				messages += `,{"content":"","role":"assistant"}`
			}
			want := `{"messages":[` + messages + `],"model":"gemini-3.1-pro-preview","stream":false}`
			if response.StatusCode != 200 || len(requests) != 1 {
				t.Fatalf("MCP result status=%d attempts=%d", response.StatusCode, len(requests))
			}
			if got := <-requests; got != want {
				t.Fatalf("MCP result wire differs: got=%s want=%s", got, want)
			}
		})
	}
}
