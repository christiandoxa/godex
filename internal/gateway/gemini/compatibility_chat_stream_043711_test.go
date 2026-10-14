package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex043711GeminiAPIKeyChatSSEKeepsTextAndExactlyOneTerminalEvent(t *testing.T) {
	for _, tc := range []struct {
		name, wire string
		events     []string
		wantText   string
	}{
		{
			name: "chat text then duplicate terminal markers",
			wire: "data: " + `{"id":"chat-1","choices":[{"delta":{"content":"synthetic-"},"finish_reason":null}]}` + "\n\n" +
				"data: " + `{"id":"chat-1","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n" +
				"data: [DONE]\n\n",
			events: []string{"response.created", "response.output_item.added", "response.output_text.delta",
				"response.output_text.delta", "response.output_item.done", "response.completed"},
			wantText: "synthetic-ok",
		},
		{
			name:     "unterminated upstream stream fails closed",
			wire:     "data: " + `{"id":"chat-2","choices":[{"delta":{"content":"partial"},"finish_reason":null}]}` + "\n\n",
			events:   []string{"response.created", "response.output_item.added", "response.output_text.delta", "response.failed"},
			wantText: "partial",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, tc.wire)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{
				{ID: "key-a", Secret: "synthetic-credential"},
			}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			reply, err := pool.Execute(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Path: "/backend-api/prodex/responses",
				Body: []byte(`{"model":"gemini-3.1-pro-preview","input":"synthetic","stream":true}`),
			}, proxymodel.Account{ID: "key-a"})
			if err != nil {
				t.Fatal(err)
			}
			defer reply.Body.Close()
			wire, err := io.ReadAll(reply.Body)
			if err != nil {
				t.Fatal(err)
			}
			parser := sse.NewDecoder(1 << 20)
			events := append(parser.Feed(wire), parser.Finish()...)
			var types []string
			var rendered strings.Builder
			var final map[string]any
			for i, raw := range events {
				var event map[string]any
				if err := json.Unmarshal(raw, &event); err != nil {
					t.Fatal(err)
				}
				kind, _ := event["type"].(string)
				types = append(types, kind)
				if event["sequence_number"] != float64(i) {
					t.Fatalf("sequence %d wrong: %v", i, event)
				}
				if kind == "response.output_text.delta" {
					delta, _ := event["delta"].(string)
					rendered.WriteString(delta)
				}
				if kind == "response.completed" {
					final = event
				}
			}
			if !reflect.DeepEqual(types, tc.events) || rendered.String() != tc.wantText {
				t.Fatalf("chat-compatible stream types=%v text=%q want=%v %q wire=%s", types, rendered.String(), tc.events, tc.wantText, wire)
			}
			if final != nil {
				result, _ := final["response"].(map[string]any)
				output, _ := result["output"].([]any)
				if len(output) != 1 {
					t.Fatalf("Chat SSE successful content was dropped from final response: %+v", result)
				}
			}
		})
	}
}
