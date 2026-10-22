package gemini

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiRequestPreparationFailuresAreTerminal(t *testing.T) {
	for _, fixture := range []struct {
		name, body, message string
		status              int
	}{
		{"tools_string", `{"input":"same request","tools":"opaque"}`, "invalid_tool_declaration: Gemini request field `tools` must be an array", 400},
		{"reasoning_string", `{"input":"same request","reasoning":"opaque"}`, "upstream request failed", 502},
		{"tool_params", `{"input":"same request","tools":[{"type":"function","function":{"name":"synthetic_lookup","parameters":false}}]}`, "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object", 400},
		{"tool_missing_params", `{"input":"same request","tools":[{"type":"function","name":"synthetic_lookup"}]}`, "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required", 400},
		{"tool_missing_name", `{"input":"same request","tools":[{"type":"function","function":{"parameters":{}}}]}`, "invalid_tool_declaration: Gemini request field `tools[0].function.name` must be a non-empty string", 400},
		{"tool_description", `{"input":"same request","tools":[{"type":"function","name":"synthetic_lookup","parameters":{},"description":7}]}`, "invalid_tool_declaration: Gemini request field `tools[0].function.description` must be a string", 400},
		{"input_number", `{"input":7}`, "upstream request failed", 502},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "synthetic", Secret: "synthetic-only"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			reply, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Body: []byte(fixture.body)}, proxymodel.Account{ID: "synthetic"})
			if reply != nil {
				defer reply.Body.Close()
			}
			var failure *proxymodel.Error
			if !errors.As(err, &failure) || failure.StatusCode != fixture.status || failure.Message != fixture.message || attempts.Load() != 0 {
				t.Fatalf("request preparation lost terminal no-attempt failure: error=%v attempts=%d", err, attempts.Load())
			}
		})
	}
}
