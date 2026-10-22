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

func TestProdex04384GeminiChatMetadataPreservesCompletePipeline(t *testing.T) {
	const nativeProvider = `"gemini":{"finishReason":"STOP","usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}}`
	const streamProvider = `"gemini":{"finish_reason":"stop"}`
	const numbers = `"decimal":1.0,"exponent":1000.0,"negative_zero":-0.0,"text":"<&/🌍  ","u64":18446744073709551615`
	for _, fixture := range []struct {
		name, fields, native, stream, fallback string
		reasoning                              bool
	}{
		{name: "empty", fields: `"metadata":{}`, native: `{` + nativeProvider + `}`, stream: `{` + streamProvider + `}`, fallback: `{}`},
		{name: "provider-empty", fields: `"metadata":{"gemini":{}}`, native: `{` + nativeProvider + `}`, stream: `{` + streamProvider + `}`, fallback: `{"gemini":{}}`},
		{name: "base", fields: `"metadata":{"tag":"synthetic","gemini":{"trace":"synthetic"}}`,
			native: `{"gemini":{"finishReason":"STOP","trace":"synthetic","usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}},"tag":"synthetic"}`,
			stream: `{"gemini":{"finish_reason":"stop","trace":"synthetic"},"tag":"synthetic"}`, fallback: `{"gemini":{"trace":"synthetic"},"tag":"synthetic"}`},
		{name: "explicit-overrides", fields: `"metadata":{"client_metadata":{"old":true},"prompt_cache_key":"old","prompt_cache_retention":"old","gemini":{"prompt_cache_key":"provider"}},"client_metadata":{"new":true},"prompt_cache_key":" new ","prompt_cache_retention":" synthetic "`,
			native:   `{"client_metadata":{"new":true},"gemini":{"finishReason":"STOP","prompt_cache_key":"provider","usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}},"prompt_cache_key":" new ","prompt_cache_retention":" synthetic "}`,
			stream:   `{"client_metadata":{"new":true},"gemini":{"finish_reason":"stop","prompt_cache_key":"provider"},"prompt_cache_key":" new ","prompt_cache_retention":" synthetic "}`,
			fallback: `{"client_metadata":{"new":true},"gemini":{"prompt_cache_key":"provider"},"prompt_cache_key":" new ","prompt_cache_retention":" synthetic "}`},
		{name: "blank-cache", fields: `"metadata":{"prompt_cache_key":"base"},"prompt_cache_key":" \t\u2003\u00a0"`,
			native: `{` + nativeProvider + `,"prompt_cache_key":"base"}`, stream: `{` + streamProvider + `,"prompt_cache_key":"base"}`, fallback: `{"prompt_cache_key":"base"}`},
		{name: "empty-retention", fields: `"prompt_cache_retention":""`, native: `{` + nativeProvider + `,"prompt_cache_retention":""}`, stream: `{` + streamProvider + `,"prompt_cache_retention":""}`, fallback: `{"prompt_cache_retention":""}`},
		{name: "numbers-unicode", fields: `"metadata":{"decimal":1.0,"exponent":1e3,"negative_zero":-0,"text":"<&/🌍\u2028\u2029","u64":18446744073709551615}`,
			native: `{"decimal":1.0,"exponent":1000.0,` + nativeProvider + `,"negative_zero":-0.0,"text":"<&/🌍  ","u64":18446744073709551615}`,
			stream: `{"decimal":1.0,"exponent":1000.0,` + streamProvider + `,"negative_zero":-0.0,"text":"<&/🌍  ","u64":18446744073709551615}`, fallback: `{` + numbers + `}`},
		{name: "nested-replacement", fields: `"metadata":{"gemini":{"finishReason":{"source":"request"},"finish_reason":{"source":"request"},"usageMetadata":{"custom":1}}}`,
			native:   `{"gemini":{"finishReason":{"source":"request"},"finish_reason":{"source":"request"},"usageMetadata":{"custom":1}}}`,
			stream:   `{"gemini":{"finishReason":{"source":"request"},"finish_reason":{"source":"request"},"usageMetadata":{"custom":1}}}`,
			fallback: `{"gemini":{"finishReason":{"source":"request"},"finish_reason":{"source":"request"},"usageMetadata":{"custom":1}}}`},
		{name: "omission-overrides", fields: `"metadata":{"gemini":{"omitted_tool_choice":{"from":"wrong","reason":"wrong"},"trace":"synthetic"}},"reasoning":{"effort":"high"},"tool_choice":null`, reasoning: true,
			native:   `{"gemini":{"finishReason":"STOP","omitted_tool_choice":{"from":null,"reason":"` + geminiChatThinkingChoiceReason04384 + `"},"trace":"synthetic","usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}}}`,
			stream:   `{"gemini":{"finish_reason":"stop","omitted_tool_choice":{"from":null,"reason":"` + geminiChatThinkingChoiceReason04384 + `"},"trace":"synthetic"}}`,
			fallback: `{"gemini":{"omitted_tool_choice":{"from":null,"reason":"` + geminiChatThinkingChoiceReason04384 + `"},"trace":"synthetic"}}`},
	} {
		for _, mode := range []string{"native", "stream", "fallback", "failed-stream"} {
			t.Run(fixture.name+"/"+mode, func(t *testing.T) {
				requests := make(chan string, 8)
				stream := strings.Contains(mode, "stream")
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					if r.Method != "POST" || r.URL.Path != "/v1beta/openai/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-key" {
						t.Error("metadata changed the provider route or credential")
					}
					requests <- string(body)
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						if mode == "failed-stream" {
							_, _ = io.WriteString(w, "data: "+`{"error":{"code":"synthetic_error","message":"synthetic failure"}}`+"\n\n")
							return
						}
						_, _ = io.WriteString(w, "data: "+`{"model":"gemini-3.1-pro-preview","choices":[{"delta":{"content":"synthetic-ok"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					if mode == "fallback" {
						_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"must-not-emit"}}]}`)
						return
					}
					_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`)
				}))
				defer server.Close()
				pool, err := NewRuntimePool(server.URL+"/v1beta/openai", []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				streamValue := "false"
				if stream {
					streamValue = "true"
				}
				body := `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + streamValue + `,` + fixture.fields + `}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				payload, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				wantWire := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview"`
				if fixture.reasoning {
					wantWire += `,"reasoning_effort":"high"`
				}
				wantWire += `,"stream":` + streamValue
				if stream {
					wantWire += `,"stream_options":{"include_usage":true}`
				}
				if response.StatusCode != 200 || len(requests) != 1 || <-requests != wantWire+"}" {
					t.Fatal("metadata changed the upstream request, attempt count or status")
				}
				want := fixture.native
				if mode == "fallback" {
					want = fixture.fallback
				}
				if stream {
					want = fixture.stream
					decoder := sse.NewDecoder(1 << 20)
					var terminal []byte
					for _, raw := range append(decoder.Feed(payload), decoder.Finish()...) {
						var event struct {
							Type     string          `json:"type"`
							Response json.RawMessage `json:"response"`
						}
						if json.Unmarshal(raw, &event) != nil {
							t.Fatal("invalid metadata stream event")
						}
						if event.Type == "response.completed" || event.Type == "response.failed" {
							if terminal != nil || (event.Type == "response.failed") != (mode == "failed-stream") {
								t.Fatal("metadata changed terminal stream outcome")
							}
							terminal = event.Response
						} else if strings.Contains(string(raw), `"metadata"`) {
							t.Fatal("metadata escaped onto a nonterminal event")
						}
					}
					payload = terminal
					if mode == "failed-stream" {
						want = ""
					}
				}
				var result struct{ Metadata json.RawMessage }
				if err := json.Unmarshal(payload, &result); err != nil || string(result.Metadata) != want {
					t.Fatalf("metadata=%s want=%s parse_error=%v", result.Metadata, want, err)
				}
			})
		}
	}
}

func TestProdex04384GeminiChatRejectsInvalidMetadataBeforeDispatch(t *testing.T) {
	for _, field := range []string{"metadata", "client_metadata", "prompt_cache_key", "prompt_cache_retention", "metadata.gemini"} {
		for _, value := range []string{"null", "false", "7", "[]", `"synthetic"`} {
			if (field == "prompt_cache_key" || field == "prompt_cache_retention") && value == `"synthetic"` {
				continue
			}
			t.Run(field+"/"+value, func(t *testing.T) {
				fields := `"` + field + `":` + value
				if field == "metadata.gemini" {
					fields = `"metadata":{"gemini":` + value + `}`
				}
				pool, err := NewRuntimePool("http://127.0.0.1:1", []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, &http.Client{Transport: forbiddenMetadataTransport04384{t}})
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				body := `{"model":"gemini-3.1-pro-preview","input":"same request",` + fields + `}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				var failure *proxymodel.Error
				if response != nil || !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
					t.Fatalf("invalid metadata lost terminal preparation failure: response=%v error=%v", response, err)
				}
			})
		}
	}
}

type forbiddenMetadataTransport04384 struct{ t *testing.T }

func (transport forbiddenMetadataTransport04384) RoundTrip(*http.Request) (*http.Response, error) {
	transport.t.Error("invalid metadata reached the upstream transport")
	return nil, errors.New("unexpected metadata dispatch")
}
