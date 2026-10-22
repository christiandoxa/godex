package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiFunctionItemsUseTaggedAliasesOnWire(t *testing.T) {
	for _, fixture := range []struct{ name, identity, call, output, wireID, arguments, result string }{
		{"control", `"call_id":"synthetic-call"`, `"name":"synthetic_lookup","arguments":"{}"`, `"output":"synthetic-result"`, "synthetic-call", "{}", "synthetic-result"},
		{"id-alias", `"tool_call_id":"synthetic-call"`, `"name":"synthetic_lookup","arguments":"{}"`, `"output":"synthetic-result"`, "synthetic-call", "{}", "synthetic-result"},
		{"id-null-shadow", `"call_id":null,"id":"synthetic-call"`, `"name":"synthetic_lookup","arguments":"{}"`, `"output":"synthetic-result"`, "call_1", "{}", "synthetic-result"},
		{"name-alias", `"call_id":"synthetic-call"`, `"tool_name":"synthetic_lookup","arguments":"{}"`, `"output":"synthetic-result"`, "synthetic-call", "{}", "synthetic-result"},
		{"nested-name", `"call_id":"synthetic-call"`, `"function":{"name":"synthetic_lookup"},"arguments":"{}"`, `"output":"synthetic-result"`, "synthetic-call", "{}", "synthetic-result"},
		{"missing-arguments", `"call_id":"synthetic-call"`, `"name":"synthetic_lookup"`, `"output":"synthetic-result"`, "synthetic-call", "{}", "synthetic-result"},
		{"input-arguments", `"call_id":"synthetic-call"`, `"name":"synthetic_lookup","input":"synthetic-input"`, `"output":"synthetic-result"`, "synthetic-call", "synthetic-input", "synthetic-result"},
		{"canonical-arguments", `"call_id":"synthetic-call"`, `"name":"synthetic_lookup","arguments":{"b":1.00,"a":"<"}`, `"output":"synthetic-result"`, "synthetic-call", `{\"a\":\"<\",\"b\":1.0}`, "synthetic-result"},
		{"namespace", `"call_id":"synthetic-call"`, `"name":"synthetic_lookup","namespace":"ignored","arguments":"{}"`, `"output":"synthetic-result"`, "synthetic-call", "{}", "synthetic-result"},
		{"output-alias", `"call_id":"synthetic-call"`, `"name":"synthetic_lookup","arguments":"{}"`, `"content":"synthetic-result"`, "synthetic-call", "{}", "synthetic-result"},
		{"output-text", `"call_id":"synthetic-call"`, `"name":"synthetic_lookup","arguments":"{}"`, `"output":["first",{"text":""},{"input_text":"last"}]`, "synthetic-call", "{}", `first\n\nlast`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			requests := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				select {
				case requests <- string(body):
				default:
					t.Error("unexpected repeated request")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"synthetic-response","model":"gemini-3.1-pro-preview","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			body := `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"type":"function_call",` + fixture.identity + `,` + fixture.call + `},{"type":"function_call_output",` + fixture.identity + `,` + fixture.output + `}]}`
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Header: make(http.Header), Body: []byte(body)}, proxymodel.Account{ID: "primary"})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			want := `{"messages":[{"content":"same request","role":"user"},{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"` + fixture.arguments + `","name":"synthetic_lookup"},"id":"` + fixture.wireID + `","type":"function"}]},{"content":"` + fixture.result + `","role":"tool","tool_call_id":"` + fixture.wireID + `"}],"model":"gemini-3.1-pro-preview","stream":false}`
			if got := <-requests; got != want {
				t.Fatalf("function alias wire differs: got=%s want=%s", got, want)
			}
		})
	}
}
