package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiShellCallSeparatesAdmissionFromProjection(t *testing.T) {
	for _, fixture := range []struct {
		name, fields, identity, arguments string
		reject                            bool
	}{
		{name: "root-string", fields: `"command":"echo synthetic"`, arguments: `{\"command\":\"echo synthetic\"}`},
		{name: "action-array", fields: `"action":{"command":["echo","synthetic"]}`, arguments: `{\"command\":\"echo synthetic\"}`},
		{name: "null-id", fields: `"command":"echo synthetic"`, identity: `"call_id":null,"tool_call_id":"synthetic-call"`, arguments: `{\"command\":\"echo synthetic\"}`},
		{name: "top-array-rejected", fields: `"command":["echo","synthetic"]`, reject: true},
		{name: "nested-string-rejected", fields: `"action":{"command":"echo synthetic"}`, reject: true},
		{name: "nested-empty-rejected", fields: `"action":{"command":[]}`, reject: true},
		{name: "nested-shadows-root", fields: `"command":"echo synthetic","action":{"command":"ignored"}`, reject: true},
		{name: "action-missing-command", fields: `"command":"echo synthetic","action":{}`, reject: true},
		{name: "root-null-renders-empty", fields: `"command":null,"action":{"command":["valid"]}`, arguments: `{\"command\":\"\"}`},
		{name: "root-mixed-renders-strings", fields: `"command":["echo",7,"synthetic",null],"action":{"command":["valid"]}`, arguments: `{\"command\":\"echo synthetic\"}`},
		{name: "canonical-options", fields: `"command":"echo <&🌍","cwd":null,"timeout":1.00,"env":{"b":9007199254740993,"a":"<&"}`, arguments: `{\"command\":\"echo <&🌍\",\"cwd\":null,\"timeout\":1.0,\"env\":{\"a\":\"<&\",\"b\":9007199254740993}}`},
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
			identity, wireID := fixture.identity, "synthetic-call"
			if identity == "" {
				identity = `"call_id":"synthetic-call"`
			} else {
				wireID = "call_1"
			}
			body := `{"model":"gemini-3.1-pro-preview","input":[{"type":"local_shell_call",` + identity + `,` + fixture.fields + `},{"type":"function_call_output",` + identity + `,"output":"synthetic-result"}]}`
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
			if fixture.reject {
				if err == nil || response != nil || len(requests) != 0 {
					t.Fatalf("invalid shell history dispatched: response=%v error=%v attempts=%d", response, err, len(requests))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			want := `{"messages":[{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"` + fixture.arguments + `","name":"shell_command"},"id":"` + wireID + `","type":"function"}]},{"content":"synthetic-result","role":"tool","tool_call_id":"` + wireID + `"}],"model":"gemini-3.1-pro-preview","stream":false}`
			if response.StatusCode != 200 || len(requests) != 1 {
				t.Fatalf("shell history status=%d attempts=%d", response.StatusCode, len(requests))
			}
			if got := <-requests; got != want {
				t.Fatalf("shell history wire differs: got=%s want=%s", got, want)
			}
		})
	}
}
