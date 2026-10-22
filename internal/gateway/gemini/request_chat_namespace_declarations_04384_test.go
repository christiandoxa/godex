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

func TestProdex04384GeminiChatNamespaceDeclarationsPreserveCompletePipeline(t *testing.T) {
	const child = `{"type":"function","name":"read","parameters":{"type":"object"}}`
	for _, fixture := range []struct {
		name, namespace, children, target, description string
		status                                         int
	}{
		{name: "basic", namespace: "fs", children: child, target: "fs--read"},
		{name: "mcp", namespace: "mcp__fs", children: child, target: "mcp__fs__read"},
		{name: "trim", namespace: " \u2003fs\u00a0 ", children: strings.Replace(child, `"read"`, `" read "`, 1), target: "fs--read"},
		{name: "pair", namespace: "mcp__fs", children: strings.Replace(child, `"read"`, `"group__read"`, 1), target: "mcp__fs--group__read"},
		{name: "description", namespace: "fs", children: strings.TrimSuffix(child, "}") + `,"description":" synthetic "}`, target: "fs--read", description: `"description":" synthetic ",`},
		{name: "duplicate", namespace: "fs", children: child + "," + child, target: "fs--read"},
		{name: "empty", namespace: "fs", status: 400},
		{name: "blank-namespace", namespace: " \u2003", children: child, status: 400},
		{name: "missing-parameters", namespace: "fs", children: `{"type":"function","name":"read"}`, status: 400},
		{name: "null-parameters", namespace: "fs", children: `{"type":"function","name":"read","parameters":null}`, status: 400},
		{name: "strict-enabled", namespace: "fs", children: strings.TrimSuffix(child, "}") + `,"strict":true}`, status: 502},
		{name: "strict-string", namespace: "fs", children: strings.TrimSuffix(child, "}") + `,"strict":"true"}`, status: 502},
		{name: "description-null", namespace: "fs", children: strings.TrimSuffix(child, "}") + `,"description":null}`, status: 502},
		{name: "invalid-namespace", namespace: "invalid.name", children: child, status: 502},
		{name: "later-wrong-type", namespace: "fs", children: child + `,{"type":"other","name":"other","parameters":{}}`, status: 502},
		{name: "later-scalar", namespace: "fs", children: child + `,null`, status: 502},
		{name: "later-blank-name", namespace: "fs", children: child + `,{"type":"function","name":"","parameters":{}}`, status: 502},
	} {
		for _, stream := range []bool{false, true} {
			mode, streamValue := "native", "false"
			if stream {
				mode, streamValue = "stream", "true"
			}
			t.Run(fixture.name+"/"+mode, func(t *testing.T) {
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
				body := `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + streamValue + `,"tools":[{"type":"namespace","name":"` + fixture.namespace + `","tools":[` + fixture.children + `]}]}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if fixture.status != 0 {
					var failure *proxymodel.Error
					if response != nil || !errors.As(err, &failure) || failure.StatusCode != fixture.status || len(requests) != 0 {
						t.Fatalf("namespace lost validation stage/no-dispatch: error=%v attempts=%d", err, len(requests))
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
				wire += `,"tools":[{"function":{` + fixture.description + `"name":"` + fixture.target + `","parameters":{"type":"object"}},"type":"function"}]}`
				if response.StatusCode != 200 || len(requests) != 1 || <-requests != wire || !strings.Contains(string(payload), "synthetic-ok") {
					t.Fatal("namespace lost exact names, projection, count or output")
				}
			})
		}
	}
}
