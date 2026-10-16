package gemini

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04380GeminiAPIKeyChatRejectsInvalidFunctionToolsBeforeUpstream(t *testing.T) {
	cases := []struct{ name, tool, want string }{
		{"wrapped malformed parameters", `{"type":"function","function":{"name":"synthetic_lookup","parameters":false}}`, "tools[0].function.parameters` must be an object"},
		{"flat missing parameters", `{"type":"function","name":"synthetic_lookup"}`, "tools[0].function.parameters` is required"},
		{"wrapped missing name", `{"type":"function","function":{"parameters":{}}}`, "tools[0].function.name` must be a non-empty string"},
		{"flat wrong description", `{"type":"function","name":"synthetic_lookup","parameters":{},"description":7}`, "tools[0].function.description` must be a string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var upstream atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstream.Add(1)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"incorrect-upstream-success"}]}}]}`))
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{{ID: "synthetic", Secret: "synthetic-only"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			body := `{"model":"gemini-3.1-pro-preview","input":"synthetic request","tools":[` + tc.tool + `]}`
			response, err := pool.Execute(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Path: "/backend-api/godex/responses", Body: []byte(body),
			}, proxymodel.Account{ID: "synthetic"})
			if response != nil {
				_ = response.Body.Close()
				t.Fatalf("invalid tool was sent or turned into a response: %+v", response)
			}
			var clientErr *proxymodel.Error
			if !errors.As(err, &clientErr) || clientErr.StatusCode != 400 ||
				!strings.Contains(clientErr.Message, "invalid_tool_declaration: Gemini request field `"+tc.want) {
				t.Fatalf("wrong indexed local rejection: %+v, want %q", err, tc.want)
			}
			if upstream.Load() != 0 {
				t.Fatalf("malformed tool reached upstream %d times", upstream.Load())
			}
		})
	}
}

func TestProdex04380GeminiAPIKeyChatKeepsValidFunctionTools(t *testing.T) {
	cases := []struct{ name, tool string }{
		{"valid nested declaration", `{"type":"function","function":{"name":"synthetic_lookup","parameters":{"type":"object"}}}`},
		{"valid flat declaration", `{"type":"function","name":"synthetic_lookup","parameters":{"type":"object"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var upstream atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstream.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/v1beta/openai/chat/completions" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				tools, _ := body["tools"].([]any)
				if len(tools) != 1 {
					t.Errorf("chat-compatible function tools were silently discarded: %+v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`))
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{{ID: "synthetic", Secret: "synthetic-only"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			response, err := pool.Execute(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Path: "/backend-api/godex/responses",
				Body: []byte(`{"model":"gemini-3.1-pro-preview","input":"synthetic","tools":[` + tc.tool + `]}`),
			}, proxymodel.Account{ID: "synthetic"})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK || upstream.Load() != 1 {
				t.Fatalf("valid tool unexpectedly blocked: status=%d upstream=%d", response.StatusCode, upstream.Load())
			}
		})
	}
}
