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

func TestProdex04384GeminiChatResponseFormatPreservesCompletePipeline(t *testing.T) {
	const user = `{"content":"same request","role":"user"}`
	const jsonInstruction = `{"content":"Respond with valid JSON only.","role":"system"},`
	const reason = "Gemini OpenAI-compatible response_format supports json_object but not native JSON Schema enforcement"
	for _, fixture := range []struct {
		name, fields, input, messages, degraded string
		jsonMode, thinking, reject              bool
	}{
		{name: "text", fields: `"response_format":{"type":"text"}`},
		{name: "json", fields: `"response_format":{"type":"json"}`, jsonMode: true},
		{name: "json-object", fields: `"response_format":{"type":"json_object"}`, jsonMode: true},
		{name: "schema", fields: `"response_format":{"type":"json_schema"}`, jsonMode: true, degraded: "json_schema"},
		{name: "structured", fields: `"response_format":{"type":"structured_output","schema":false}`, jsonMode: true, degraded: "structured_output"},
		{name: "alias", fields: `"text":{"format":{"type":"json"}}`, jsonMode: true},
		{name: "top-precedence", fields: `"response_format":{"type":"text"},"text":{"format":{"type":"json_schema"}}`},
		{name: "text-null-control", fields: `"text":null`},
		{name: "invalid-shadow", fields: `"response_format":null,"text":{"format":{"type":"json"}}`, reject: true},
		{name: "no-type", fields: `"response_format":{}`, reject: true},
		{name: "wrong-type", fields: `"response_format":{"type":7}`, reject: true},
		{name: "unknown", fields: `"response_format":{"type":"JSON"}`, reject: true},
		{name: "user-guidance", fields: `"response_format":{"type":"json_object"}`, input: `"input":"jSoN"`, messages: `{"content":"jSoN","role":"user"}`, jsonMode: true},
		{name: "system-guidance", fields: `"response_format":{"type":"json_object"}`, input: `"input":"same request","instructions":"Return JSON"`, messages: `{"content":"Return JSON","role":"system"},` + user, jsonMode: true},
		{name: "developer-guidance", fields: `"response_format":{"type":"json_object"}`, input: `"input":[{"type":"message","role":"developer","content":"Return JSON"}]`, messages: `{"content":"Return JSON","role":"system"}`, jsonMode: true},
		{name: "assistant-cannot-suppress", fields: `"response_format":{"type":"json_object"}`, input: `"input":[{"type":"message","role":"assistant","content":"JSON"}]`, messages: jsonInstruction + `{"content":"JSON","role":"assistant"}`, jsonMode: true},
		{name: "split-cannot-suppress", fields: `"response_format":{"type":"json_object"}`, input: `"input":[{"type":"message","role":"user","content":[{"text":"JS"},{"text":"ON"}]}]`, messages: jsonInstruction + `{"content":"JS\nON","role":"user"}`, jsonMode: true},
		{name: "generated-notes", fields: `"response_format":{"type":"json_schema"},"metadata":{"gemini":{"degraded_response_format":{"from":"wrong"},"omitted_tool_choice":{"from":"wrong"}}},"reasoning":{"effort":"high"},"tool_choice":null`, jsonMode: true, degraded: "json_schema", thinking: true},
	} {
		modes := []string{"native", "stream", "fallback"}
		if fixture.reject {
			modes = []string{"native"}
		}
		for _, mode := range modes {
			t.Run(fixture.name+"/"+mode, func(t *testing.T) {
				requests := make(chan string, 8)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					requests <- string(body)
					if mode == "stream" {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+`{"model":"gemini-3.1-pro-preview","choices":[{"delta":{"content":"synthetic-ok"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					if mode == "fallback" {
						_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"must-not-emit"}}]}`)
						return
					}
					_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}]}`)
				}))
				defer server.Close()
				pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				input := fixture.input
				if input == "" {
					input = `"input":"same request"`
				}
				stream := "false"
				if mode == "stream" {
					stream = "true"
				}
				body := `{"model":"gemini-3.1-pro-preview",` + input + `,"stream":` + stream + `,` + fixture.fields + `}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if fixture.reject {
					var failure *proxymodel.Error
					if response != nil || !errors.As(err, &failure) || failure.StatusCode != 502 || len(requests) != 0 {
						t.Fatal("invalid format lost terminal no-dispatch failure")
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
				messages := fixture.messages
				if messages == "" {
					messages = user
					if fixture.jsonMode {
						messages = jsonInstruction + messages
					}
				}
				wire := `{"messages":[` + messages + `],"model":"gemini-3.1-pro-preview"`
				if fixture.thinking {
					wire += `,"reasoning_effort":"high"`
				}
				if fixture.jsonMode {
					wire += `,"response_format":{"type":"json_object"}`
				}
				wire += `,"stream":` + stream
				if mode == "stream" {
					wire += `,"stream_options":{"include_usage":true}`
				}
				if len(requests) != 1 || <-requests != wire+"}" || response.StatusCode != 200 {
					t.Fatal("format changed exact guidance, control, request count or status")
				}
				if mode == "stream" {
					decoder := sse.NewDecoder(1 << 20)
					var completed []byte
					for _, raw := range append(decoder.Feed(payload), decoder.Finish()...) {
						if strings.Contains(string(raw), `"type":"response.completed"`) {
							var event struct{ Response json.RawMessage }
							if json.Unmarshal(raw, &event) != nil || completed != nil {
								t.Fatal("format corrupted completion")
							}
							completed = event.Response
						}
					}
					payload = completed
				}
				var result struct {
					Metadata struct{ Gemini map[string]json.RawMessage }
				}
				if json.Unmarshal(payload, &result) != nil {
					t.Fatal("invalid formatted response")
				}
				want := ""
				if fixture.degraded != "" {
					want = `{"from":"` + fixture.degraded + `","reason":"` + reason + `","to":"json_object"}`
				}
				if string(result.Metadata.Gemini["degraded_response_format"]) != want {
					t.Fatal("format lost exact degradation metadata")
				}
				if fixture.thinking {
					want := `{"from":null,"reason":"` + geminiChatThinkingChoiceReason04384 + `"}`
					if string(result.Metadata.Gemini["omitted_tool_choice"]) != want {
						t.Fatal("format lost thinking omission metadata")
					}
				}
			})
		}
	}
}
