package gemini

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiChatReasoningPreservesCompletePipeline(t *testing.T) {
	for _, fixture := range []struct {
		name, fields, effort, choice string
		reject                       bool
	}{
		{name: "empty", fields: `"reasoning":{}`},
		{name: "low", fields: `"reasoning":{"effort":"low"}`, effort: "low"},
		{name: "alias-high", fields: `"reasoning_effort":" \tHiGh\u2003"`, effort: "high"},
		{name: "xhigh", fields: `"reasoning_effort":"xhigh"`, effort: "high"},
		{name: "minimal", fields: `"reasoning":{"effort":"minimal"}`, effort: "minimal", choice: `"auto"`},
		{name: "none", fields: `"reasoning":{"effort":"none"}`, effort: "none"},
		{name: "nested-shadow", fields: `"reasoning":{"effort":"medium"},"reasoning_effort":false`, effort: "medium"},
		{name: "choice-auto", fields: `"reasoning":{"effort":"high"}`, effort: "high", choice: `"auto"`},
		{name: "choice-null", fields: `"reasoning":{"effort":"high"}`, effort: "high", choice: "null"},
		{name: "choice-false", fields: `"reasoning":{"effort":"high"}`, effort: "high", choice: "false"},
		{name: "null-rejected", fields: `"reasoning":null`, reject: true},
		{name: "extra-key-rejected", fields: `"reasoning":{"effort":"low","summary":"auto"}`, reject: true},
		{name: "alias-type-rejected", fields: `"reasoning_effort":false`, reject: true},
		{name: "unicode-rejected", fields: `"reasoning_effort":"hİgh"`, reject: true},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fixture.name+"/stream_"+strconv.FormatBool(stream), func(t *testing.T) {
				requests := make(chan string, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					requests <- string(body)
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+`{"model":"gemini-3.1-pro-preview","choices":[{"delta":{"content":"synthetic-ok"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
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
				fields := fixture.fields
				if fixture.choice != "" {
					fields += `,"tool_choice":` + fixture.choice
				}
				body := `{"model":"gemini-3.1-pro-preview","stream":` + strconv.FormatBool(stream) + `,` + fields + `,"input":[{"type":"function_call","call_id":"synthetic-call","name":"synthetic_lookup","arguments":"{}"},{"type":"function_call_output","call_id":"synthetic-call","output":"synthetic-result"}]}`
				response, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
					Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "primary"})
				if fixture.reject {
					var failure *proxymodel.Error
					if !errors.As(err, &failure) || failure.StatusCode != 502 || response != nil || len(requests) != 0 {
						t.Fatalf("invalid reasoning dispatched: error=%v response=%v attempts=%d", err, response, len(requests))
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				thinking := fixture.effort == "high" || fixture.effort == "medium" || fixture.effort == "low"
				reasoning := ""
				if thinking {
					reasoning = `,"reasoning_content":""`
				}
				want := `{"messages":[{"content":""` + reasoning + `,"role":"assistant","tool_calls":[{"function":{"arguments":"{}","name":"synthetic_lookup"},"id":"synthetic-call","type":"function"}]},{"content":"synthetic-result","role":"tool","tool_call_id":"synthetic-call"}],"model":"gemini-3.1-pro-preview"`
				if fixture.effort != "" {
					want += `,"reasoning_effort":"` + fixture.effort + `"`
				}
				want += `,"stream":` + strconv.FormatBool(stream)
				if stream {
					want += `,"stream_options":{"include_usage":true}`
				}
				if fixture.choice != "" && !thinking {
					want += `,"tool_choice":` + fixture.choice
				}
				if response.StatusCode != 200 || len(requests) != 1 || stream && response.Header.Get("Content-Type") != "text/event-stream; charset=utf-8" {
					t.Fatalf("reasoning status=%d attempts=%d", response.StatusCode, len(requests))
				}
				if got := <-requests; got != want+"}" {
					t.Fatalf("reasoning wire differs: got=%s want=%s", got, want+"}")
				}
				payload, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if stream {
					decoder := sse.NewDecoder(1 << 20)
					var completed []byte
					for _, raw := range append(decoder.Feed(payload), decoder.Finish()...) {
						if strings.Contains(string(raw), `"type":"response.completed"`) {
							var event map[string]json.RawMessage
							if json.Unmarshal(raw, &event) != nil || completed != nil {
								t.Fatal("invalid or duplicate terminal event")
							}
							completed = event["response"]
						}
					}
					payload = completed
				}
				var result struct {
					Metadata struct{ Gemini map[string]json.RawMessage }
				}
				if err := json.Unmarshal(payload, &result); err != nil {
					t.Fatal(err)
				}
				omitted := string(result.Metadata.Gemini["omitted_tool_choice"])
				wantNote := ""
				if thinking && fixture.choice != "" {
					wantNote = `{"from":` + fixture.choice + `,"reason":"Gemini OpenAI-compatible thinking mode currently rejects explicit tool_choice on the OpenAI Chat route, so Prodex omits it while preserving translated function tools"}`
				}
				if omitted != wantNote {
					t.Fatalf("reasoning omission metadata=%s want=%s", omitted, wantNote)
				}
			})
		}
	}
}
