package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiCustomCallsUseTaggedProjection(t *testing.T) {
	for _, fixture := range []struct {
		name, fields, arguments string
		empty                   bool
	}{
		{name: "string", fields: `"name":"synthetic_lookup","input":"synthetic-input"`, arguments: `{\"input\":\"synthetic-input\"}`},
		{name: "missing", fields: `"name":"synthetic_lookup","arguments":"ignored"`, arguments: `{\"input\":\"\"}`},
		{name: "null", fields: `"name":"synthetic_lookup","input":null`, arguments: `{\"input\":\"null\"}`},
		{name: "object", fields: `"name":"synthetic_lookup","input":{"b":1.00,"a":"<"}`, arguments: `{\"input\":\"{\\\"a\\\":\\\"<\\\",\\\"b\\\":1.0}\"}`},
		{name: "array", fields: `"name":"synthetic_lookup","input":["<",1.00,null]`, arguments: `{\"input\":\"[\\\"<\\\",1.0,null]\"}`},
		{name: "name-null-shadow", fields: `"name":null,"tool_name":"synthetic_lookup","input":"ignored"`, empty: true},
		{name: "name-nested", fields: `"function":{"name":"synthetic_lookup"}`, empty: true},
		{name: "name-sentinel", fields: `"__missing__":"synthetic_lookup"`, empty: true},
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
			body := `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"type":"custom_tool_call","call_id":"synthetic-call",` + fixture.fields + `},{"type":"function_call_output","call_id":"synthetic-call","output":"synthetic-result"}]}`
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			messages := `{"content":"same request","role":"user"},{"content":"","role":"assistant"}`
			if !fixture.empty {
				messages = `{"content":"same request","role":"user"},{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"` + fixture.arguments + `","name":"synthetic_lookup"},"id":"synthetic-call","type":"function"}]},{"content":"synthetic-result","role":"tool","tool_call_id":"synthetic-call"}`
			}
			want := `{"messages":[` + messages + `],"model":"gemini-3.1-pro-preview","stream":false}`
			if response.StatusCode != 200 || len(requests) != 1 {
				t.Fatalf("custom call status=%d attempts=%d", response.StatusCode, len(requests))
			}
			if got := <-requests; got != want {
				t.Fatalf("custom call wire differs: got=%s want=%s", got, want)
			}
		})
	}
}
