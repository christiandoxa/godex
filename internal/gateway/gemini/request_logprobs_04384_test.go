package gemini

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

func TestProdex04384GeminiLogprobsAdmissionAndWire(t *testing.T) {
	for _, fixture := range []struct{ name, fields, prefix, suffix string }{
		{"false", `"logprobs":false`, `"logprobs":false,`, `,"stream":false`},
		{"zero", `"logprobs":true,"top_logprobs":0`, `"logprobs":true,`, `,"stream":false,"top_logprobs":0`},
		{"maximum", `"logprobs":true,"top_logprobs":20`, `"logprobs":true,`, `,"stream":false,"top_logprobs":20`},
		{"flag_only", `"logprobs":true`, `"logprobs":true,`, `,"stream":false`},
		{"stream_and_identity", `"logprobs":true,"top_logprobs":2,"stream":true,"user_id":"synthetic_user"`, `"logprobs":true,`, `,"stream":true,"stream_options":{"include_usage":true},"top_logprobs":2,"user_id":"synthetic_user"`},
		{"null_flag", `"logprobs":null`, "", ""},
		{"string_flag", `"logprobs":"true"`, "", ""},
		{"null_top", `"logprobs":true,"top_logprobs":null`, "", ""},
		{"negative", `"logprobs":true,"top_logprobs":-1`, "", ""},
		{"negative_zero", `"logprobs":true,"top_logprobs":-0`, "", ""},
		{"fraction", `"logprobs":true,"top_logprobs":1.0`, "", ""},
		{"exponent", `"logprobs":true,"top_logprobs":1e0`, "", ""},
		{"over_maximum", `"logprobs":true,"top_logprobs":21`, "", ""},
		{"unsigned", `"logprobs":true,"top_logprobs":18446744073709551615`, "", ""},
		{"missing_flag", `"top_logprobs":2`, "", ""},
		{"false_flag", `"logprobs":false,"top_logprobs":0`, "", ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var attempts atomic.Int32
			captured := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				captured <- string(body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]}}]}`)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "synthetic", Secret: "synthetic-only"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			reply, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Body: []byte(`{"model":"gemini-3.1-pro-preview","input":"same request",` + fixture.fields + `}`)}, proxymodel.Account{ID: "synthetic"})
			if reply != nil {
				defer reply.Body.Close()
			}
			if fixture.prefix == "" {
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" || attempts.Load() != 0 {
					t.Fatalf("invalid logprobs lost terminal no-attempt admission: error=%v attempts=%d", err, attempts.Load())
				}
				return
			}
			if err != nil || reply == nil || attempts.Load() != 1 {
				t.Fatalf("valid logprobs did not reach upstream once: error=%v attempts=%d", err, attempts.Load())
			}
			want := `{` + fixture.prefix + `"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview"` + fixture.suffix + `}`
			if body := <-captured; body != want {
				t.Fatalf("logprobs request bytes = %s, want %s", body, want)
			}
			body, err := io.ReadAll(reply.Body)
			if err != nil || reply.StatusCode != 200 || !strings.Contains(string(body), "synthetic-ok") {
				t.Fatal("valid logprobs lost native response")
			}
		})
	}
}
