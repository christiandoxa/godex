package gemini

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiChatToolChoicePreservesCompletePipeline(t *testing.T) {
	for _, fixture := range []struct {
		name, choice, target      string
		reject, noTools, thinking bool
	}{
		{name: "auto", choice: `"auto"`},
		{name: "none", choice: `"none"`},
		{name: "required", choice: `"required"`},
		{name: "null", choice: "null", reject: true},
		{name: "false", choice: "false", reject: true},
		{name: "number", choice: "7", reject: true},
		{name: "array", choice: "[]", reject: true},
		{name: "unsupported", choice: `"unsupported"`, reject: true},
		{name: "empty-object", choice: `{}`, reject: true},
		{name: "type-null", choice: `{"type":null,"name":"synthetic_lookup"}`, reject: true},
		{name: "flat", choice: `{"type":"function","name":"synthetic_lookup"}`},
		{name: "nested", choice: `{"type":"function","function":{"name":"synthetic_lookup"}}`},
		{name: "top-precedence", choice: `{"type":"function","name":"synthetic_lookup","function":{"name":"wrong"}}`},
		{name: "type-fallback", choice: `{"type":"function","name":false,"function":{"name":"synthetic_lookup"}}`},
		{name: "empty-shadow", choice: `{"type":"function","name":"","function":{"name":"synthetic_lookup"}}`, reject: true},
		{name: "blank-shadow", choice: `{"type":"function","name":" \u2003","function":{"name":"synthetic_lookup"}}`, reject: true},
		{name: "unbound", choice: `{"type":"function","name":"missing"}`, reject: true},
		{name: "no-tools", choice: `{"type":"function","name":"synthetic_lookup"}`, reject: true, noTools: true},
		{name: "extra", choice: `{"type":"function","name":"synthetic_lookup","extra":false}`},
		{name: "namespace", choice: `{"type":"function","namespace":" \u2003fs\u00a0 ","name":" lookup "}`, target: "fs--lookup"},
		{name: "mcp-namespace", choice: `{"type":"mcp_tool","server_label":"fs","name":"lookup"}`, target: "mcp__fs__lookup"},
		{name: "mcp-prefix", choice: `{"type":"mcp_tool","namespace":"mcp__fs","name":"lookup"}`, target: "mcp__fs__lookup"},
		{name: "mcp-pair", choice: `{"type":"mcp_tool","namespace":"fs","name":"group__lookup"}`, target: "mcp__fs--group__lookup"},
		{name: "mcp-suffix", choice: `{"type":"mcp_tool","namespace":"fs_","name":"lookup"}`, target: "mcp__fs_--lookup"},
		{name: "mcp-leading-name", choice: `{"type":"mcp_tool","namespace":"fs","name":"_lookup"}`, target: "mcp__fs--_lookup"},
		{name: "empty-namespace-shadow", choice: `{"type":"function","namespace":"","server_label":"wrong","name":"synthetic_lookup"}`},
		{name: "namespace-type-fallback", choice: `{"type":"function","namespace":null,"server_label":"fs","name":"lookup"}`, target: "fs--lookup"},
		{name: "nested-namespace", choice: `{"type":"function","function":{"namespace":"fs","name":"lookup"}}`, target: "fs--lookup"},
		{name: "mcp-raw-leading", choice: `{"type":"mcp_tool","namespace":" fs ","name":"lookup"}`, target: "mcp__fs__lookup", reject: true},
		{name: "limit", choice: `{"type":"function","name":"` + strings.Repeat("n", 128) + `"}`, target: strings.Repeat("n", 128)},
		{name: "over", choice: `{"type":"function","name":"` + strings.Repeat("n", 129) + `"}`, reject: true},
		{name: "unicode", choice: `{"type":"function","name":"検索"}`, reject: true},
		{name: "thinking-bypasses-invalid", choice: `{"type":"other","name":false}`, thinking: true},
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
				target := fixture.target
				if target == "" {
					target = "synthetic_lookup"
				}
				tools := `[{"function":{"description":"synthetic tool","name":"` + target + `","parameters":{"type":"object"}},"type":"function"}]`
				body := `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + streamValue + `,"tool_choice":` + fixture.choice
				wire := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview"`
				if fixture.thinking {
					body += `,"reasoning":{"effort":"high"}`
					wire += `,"reasoning_effort":"high"`
				}
				wire += `,"stream":` + streamValue
				if stream {
					wire += `,"stream_options":{"include_usage":true}`
				}
				if !fixture.thinking {
					choice := `{"function":{"name":"` + target + `"},"type":"function"}`
					if strings.HasPrefix(fixture.choice, `"`) {
						choice = fixture.choice
					}
					wire += `,"tool_choice":` + choice
				}
				if !fixture.noTools {
					body += `,"tools":` + tools
					wire += `,"tools":` + tools
				}
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body + "}")}, proxymodel.Account{ID: "primary"})
				if fixture.reject {
					var failure *proxymodel.Error
					if response != nil || !errors.As(err, &failure) || failure.StatusCode != 502 || len(requests) != 0 {
						t.Fatal("invalid tool choice lost terminal no-dispatch failure")
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
				if response.StatusCode != 200 || len(requests) != 1 || <-requests != wire+"}" {
					t.Fatal("tool choice lost exact projection, target, request count or status")
				}
				if stream {
					decoder := sse.NewDecoder(1 << 20)
					var completed []byte
					for _, raw := range append(decoder.Feed(payload), decoder.Finish()...) {
						if strings.Contains(string(raw), `"type":"response.completed"`) {
							var event struct{ Response json.RawMessage }
							if json.Unmarshal(raw, &event) != nil || completed != nil {
								t.Fatal("tool choice corrupted completion")
							}
							completed = event.Response
						}
					}
					payload = completed
				}
				if fixture.thinking && !strings.Contains(string(payload), `"omitted_tool_choice":{"from":{"name":false,"type":"other"},"reason":"`+geminiChatThinkingChoiceReason04384+`"}`) {
					t.Fatal("tool choice lost the original omitted value")
				}
			})
		}
	}
}
