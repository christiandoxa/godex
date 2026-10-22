package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiToolOutputKindsShareWireProjection(t *testing.T) {
	for _, kind := range []string{"function_call_output", "custom_tool_call_output", "mcp_tool_result", "mcp_call_output"} {
		for _, fixture := range []struct{ name, output, want string }{
			{"string", `"output":"synthetic-result"`, "synthetic-result"},
			{"null", `"output":null,"content":"ignored"`, "null"},
			{"false", `"output":false`, "false"},
			{"empty-array", `"output":[]`, ""},
			{"text-parts", `"output":["first",{"text":""},{"input_text":"last"}]`, `first\n\nlast`},
			{"object", `"output":{"b":1.00,"a":"<"}`, `{\"a\":\"<\",\"b\":1.0}`},
			{"error", `"error":"synthetic-result"`, "synthetic-result"},
		} {
			t.Run(kind+"/"+fixture.name, func(t *testing.T) {
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
				body := `{"model":"gemini-3.1-pro-preview","input":[{"type":"function_call","call_id":"synthetic-call","name":"synthetic_lookup","arguments":"{}"},{"type":"` + kind + `","call_id":"synthetic-call",` + fixture.output + `}]}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
					Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				want := `{"messages":[{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"{}","name":"synthetic_lookup"},"id":"synthetic-call","type":"function"}]},{"content":"` + fixture.want + `","role":"tool","tool_call_id":"synthetic-call"}],"model":"gemini-3.1-pro-preview","stream":false}`
				if response.StatusCode != 200 || len(requests) != 1 {
					t.Fatalf("tool output status=%d attempts=%d", response.StatusCode, len(requests))
				}
				if got := <-requests; got != want {
					t.Fatalf("tool output wire differs: got=%s want=%s", got, want)
				}
			})
		}
	}
}
