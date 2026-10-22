package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiCompactReplaysBufferedChatHistory(t *testing.T) {
	for _, fixture := range []struct {
		name, extra, seedInput, compactInput, account string
		failed                                        bool
		history                                       []any
	}{
		{name: "previous", history: []any{map[string]any{"role": "user", "content": "same request"}}},
		{name: "assistant", extra: `,"choices":[{"message":{"role":"user","content":"cached assistant"}}]`, history: []any{
			map[string]any{"role": "user", "content": "same request"}, map[string]any{"role": "assistant", "content": "cached assistant"},
		}},
		{name: "instructions", seedInput: `,"instructions":"seed instruction"`, history: []any{
			map[string]any{"role": "system", "content": "seed instruction"}, map[string]any{"role": "user", "content": "same request"},
		}},
		{name: "duplicate input", compactInput: `[{"type":"message","role":"user","content":"same request"}]`, history: []any{map[string]any{"role": "user", "content": "same request"}}},
		{name: "different credential", account: "secondary"},
		{name: "failed seed", failed: true},
		{name: "ignored assistant metadata", extra: `,"choices":[{"message":{"content":"cached assistant","annotations":[{"value":9007199254740993}]}}]`, history: []any{map[string]any{"role": "user", "content": "same request"}, map[string]any{"role": "assistant", "content": "cached assistant"}}},
		{name: "tool call number", extra: `,"choices":[{"message":{"tool_calls":[{"id":"call-synthetic","type":"function","priority":9007199254740993,"function":{"name":"lookup","arguments":"{}"}}]}}]`, compactInput: `[{"type":"function_call_output","call_id":"call-synthetic","output":"done"}]`, history: []any{
			map[string]any{"role": "user", "content": "same request"}, map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "call-synthetic", "type": "function", "priority": json.Number("9007199254740993"), "function": map[string]any{"name": "lookup", "arguments": "{}"}}}}, map[string]any{"role": "tool", "tool_call_id": "call-synthetic", "content": "done"},
		}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			requests := make(chan map[string]any, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				decoder := json.NewDecoder(r.Body)
				decoder.UseNumber()
				if err := decoder.Decode(&request); err != nil {
					t.Error(err)
				}
				requests <- request
				w.Header().Set("Content-Type", "application/json")
				body := `{"id":"upstream-id","responseId":"resp-synthetic","candidates":[{"content":{"parts":[{"text":"synthetic-ok"}]}}]`
				if request["model"] == "gemini-3.8-flash" {
					body += `}`
				} else if fixture.failed {
					body = `{"id":"upstream-id","responseId":"resp-synthetic","candidates":[]}`
				} else {
					body += fixture.extra + `}`
				}
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-primary"}, {ID: "secondary", Secret: "synthetic-secondary"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			seed := `{"model":"gemini-3.1-pro-preview","input":"same request"` + fixture.seedInput + `}`
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: mountPath + "/responses", Body: []byte(seed)}, proxymodel.Account{ID: "primary"})
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			input := fixture.compactInput
			if input == "" {
				input = `[]`
			}
			account := fixture.account
			if account == "" {
				account = "primary"
			}
			body := `{"model":"gemini-3.8-flash","previous_response_id":"resp-synthetic","input":` + input + `}`
			response, err = pool.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: mountPath + "/responses/compact", Body: []byte(body)}, proxymodel.Account{ID: account})
			if err != nil {
				t.Fatal(err)
			}
			content, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil || response.StatusCode != 200 || !strings.Contains(string(content), "synthetic-ok") {
				t.Fatalf("Compact response: status=%d error=%v", response.StatusCode, err)
			}
			if len(requests) != 2 {
				t.Fatal("Compact changed attempt count")
			}
			<-requests
			actual := <-requests
			want := append([]any{map[string]any{"role": "system", "content": compactInstructions}}, fixture.history...)
			want = append(want, map[string]any{"role": "user", "content": compactInstructions})
			if !reflect.DeepEqual(actual["messages"], want) {
				t.Fatalf("Compact messages=%#v, want %#v", actual["messages"], want)
			}
			if _, exists := actual["previous_response_id"]; exists {
				t.Fatal("private continuation ID reached upstream")
			}
		})
	}
}
