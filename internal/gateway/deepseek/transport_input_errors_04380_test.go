package deepseek

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04380DeepSeekInvalidInputIsTerminalBeforeUpstream(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-input-fixture","choices":[{"message":{"role":"assistant","content":"accepted"}}]}`)
	}))
	defer upstream.Close()
	for _, fixture := range []struct{ name, body string }{
		{"null-content", `{"input":[{"type":"message","role":"user","content":null}]}`},
		{"oversized", `{"input":"` + strings.Repeat("x", 4<<20) + `"}`},
		{"null-type", `{"input":[{"type":null,"content":"note"}]}`},
		{"numeric-type", `{"input":[{"type":1,"content":"note"}]}`},
		{"empty-type", `{"input":[{"type":"","content":"note"}]}`},
		{"empty-tool-name", `{"input":[{"type":"function_call","call_id":"call-fixture","name":"","function":{"name":"lookup"}}]}`},
		{"invalid-shell-action", `{"input":[{"type":"local_shell_call","call_id":"call-fixture","action":{"command":[]},"command":"fallback"}]}`},
		{"blank-shell-id", `{"input":[{"type":"local_shell_call","call_id":"  ","action":{"command":["echo","ok"]}}]}`},
		{"zero-token-limit", `{"input":"same request","max_tokens":0}`},
		{"overflow-token-limit", `{"input":"same request","max_tokens":18446744073709551616}`},
		{"shadow-null-token-limit", `{"input":"same request","max_output_tokens":null,"max_completion_tokens":31}`},
		{"shadow-zero-token-limit", `{"input":"same request","max_tokens":0,"max_completion_tokens":31}`},
		{"shadow-fraction-token-limit", `{"input":"same request","max_output_tokens":1.0,"max_completion_tokens":31}`},
		{"null-temperature", `{"input":"same request","temperature":null}`},
		{"string-temperature", `{"input":"same request","temperature":"0.2"}`},
		{"boolean-top-p", `{"input":"same request","top_p":false}`},
		{"array-top-p", `{"input":"same request","top_p":[]}`},
		{"null-logprobs", `{"input":"same request","logprobs":null}`},
		{"string-logprobs", `{"input":"same request","logprobs":"true"}`},
		{"null-top-logprobs", `{"input":"same request","logprobs":true,"top_logprobs":null}`},
		{"negative-top-logprobs", `{"input":"same request","logprobs":true,"top_logprobs":-1}`},
		{"fraction-top-logprobs", `{"input":"same request","logprobs":true,"top_logprobs":1.0}`},
		{"large-top-logprobs", `{"input":"same request","logprobs":true,"top_logprobs":21}`},
		{"unsigned-top-logprobs", `{"input":"same request","logprobs":true,"top_logprobs":18446744073709551615}`},
		{"missing-logprobs-flag", `{"input":"same request","top_logprobs":2}`},
		{"false-logprobs-flag", `{"input":"same request","logprobs":false,"top_logprobs":0}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			calls.Store(0)
			transport, err := NewRuntimeTransport(upstream.URL, "synthetic-input-key", upstream.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			response, err := transport.Execute(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Path: mountPath + "/responses", Body: []byte(fixture.body),
			}, proxymodel.Account{})
			if response != nil {
				_ = response.Body.Close()
			}
			var presentation *proxymodel.Error
			if !errors.As(err, &presentation) || presentation.StatusCode != http.StatusBadGateway ||
				presentation.Message != "upstream request failed" {
				t.Errorf("invalid input error = %v; want terminal generic 502", err)
			}
			if got := calls.Load(); got != 0 {
				t.Errorf("invalid input reached upstream %d times", got)
			}
		})
	}
}
