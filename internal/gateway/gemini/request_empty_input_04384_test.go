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

func TestProdex04384GeminiEmptyInputUsesTaggedUserFallback(t *testing.T) {
	for _, fixture := range []struct{ name, fields, messages string }{
		{"absent", "", `[{"content":"","role":"user"}]`},
		{"array", `,"input":[]`, `[{"content":"","role":"user"}]`},
		{"whitespace", `,"instructions":" \t\u2003"`, `[{"content":"","role":"user"}]`},
		{"blank-instructions", `,"instructions":"","input":[]`, `[{"content":"","role":"user"}]`},
		{"empty-string", `,"input":""`, `[{"content":"","role":"user"}]`},
		{"system", `,"instructions":"same request","input":[]`, `[{"content":"same request","role":"system"}]`},
		{"null", `,"input":null`, ""},
		{"object", `,"input":{}`, ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var attempts atomic.Int32
			requests := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				select {
				case requests <- string(body):
				default:
					t.Error("unexpected repeated request")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"synthetic-response","model":"gemini-3.1-pro-preview","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-key"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			response, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
				Path: "/backend-api/godex/responses", Header: make(http.Header), Body: []byte(`{"model":"gemini-3.1-pro-preview"` + fixture.fields + `}`)}, proxymodel.Account{ID: "primary"})
			if response != nil {
				defer response.Body.Close()
			}
			if fixture.messages == "" {
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" || attempts.Load() != 0 {
					t.Fatalf("invalid input changed admission: error=%v attempts=%d", err, attempts.Load())
				}
				return
			}
			if err != nil || response == nil || response.StatusCode != 200 || attempts.Load() != 1 {
				t.Fatalf("empty input lost tagged fallback: error=%v attempts=%d", err, attempts.Load())
			}
			want := `{"messages":` + fixture.messages + `,"model":"gemini-3.1-pro-preview","stream":false}`
			if got := <-requests; got != want {
				t.Fatalf("empty-input wire differs: got=%s want=%s", got, want)
			}
		})
	}
}
