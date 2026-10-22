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

func TestProdex04384GeminiChatProjectsMessageContentBeforeSending(t *testing.T) {
	for _, fixture := range []struct{ name, fields, messages string }{
		{"control", `"input":[{"type":"message","role":"user","content":"same request"}]`, `[{"content":"same request","role":"user"}]`},
		{"string-array", `"input":[{"type":"message","role":"user","content":["same request"]}]`, `[{"content":"same request","role":"user"}]`},
		{"alias-array", `"input":[{"type":"message","role":"user","content":[{"input_text":"same request"}]}]`, `[{"content":"same request","role":"user"}]`},
		{"empty-separators", `"input":[{"type":"message","role":"user","content":["first",{"text":""},{"output_text":"last"}]}]`, `[{"content":"first\n\nlast","role":"user"}]`},
		{"object", `"input":[{"type":"message","role":"user","content":{"type":"input_text","text":"same request"}}]`, `[{"content":"{\"text\":\"same request\",\"type\":\"input_text\"}","role":"user"}]`},
		{"empty-message", `"input":[{"type":"message","role":"user","content":""}]`, `[{"content":"","role":"user"}]`},
		{"system-only", `"input":[{"type":"message","role":"system","content":"same request"}]`, `[{"content":"same request","role":"system"}]`},
		{"developer", `"input":[{"type":"message","role":"developer","content":"same request"}]`, `[{"content":"same request","role":"system"}]`},
		{"instructions-only", `"instructions":"same request"`, `[{"content":"same request","role":"system"}]`},
		{"null-input", `"instructions":"same request","input":null`, ""},
		{"cache-control", `"input":[{"type":"message","role":"user","content":[{"text":"same request","cache_control":null}]}]`, ""},
		{"alias-priority", `"input":[{"type":"message","role":"user","content":[{"text":7,"input_text":"same request"}]}]`, `[{"content":"same request","role":"user"}]`},
		{"implicit-default", `"input":[{"role":7,"content":"same request"}]`, `[{"content":"same request","role":"user"}]`},
		{"implicit-unknown", `"input":[{"role":"unknown","content":"same request"}]`, `[{"content":"same request","role":"user"}]`},
		{"empty-object", `"input":[{}]`, `[{"content":"","role":"user"}]`},
		{"whitespace-instructions", `"input":"same request","instructions":" \t\u2003"`, `[{"content":"same request","role":"user"}]`},
		{"scalar-parts", `"input":[{"type":"message","content":[7,null,false,"same request"]}]`, `[{"content":"same request","role":"user"}]`},
		{"prefix-part", `"input":[{"type":"message","content":[{"text":"same request","prefix":null}]}]`, `[{"content":"same request","role":"user"}]`},
		{"invalid-part", `"input":[{"type":"message","content":[{"ignored":true}]},{"role":"user","content":"same request"}]`, ""},
		{"media-ignored", `"input":[{"type":"message","content":[{"type":"input_image","image_url":"synthetic-image"},{"text":"same request"}]}]`, `[{"content":"same request","role":"user"}]`},
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
					t.Error("unexpected repeated upstream attempt")
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
				Path: "/backend-api/godex/responses", Header: make(http.Header),
				Body: []byte(`{"model":"gemini-3.1-pro-preview",` + fixture.fields + `}`)}, proxymodel.Account{ID: "primary"})
			if response != nil {
				defer response.Body.Close()
			}
			if fixture.messages == "" {
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" || attempts.Load() != 0 {
					t.Fatalf("invalid input was not terminal before upstream: error=%v attempts=%d", err, attempts.Load())
				}
				return
			}
			if err != nil || response == nil || response.StatusCode != 200 || attempts.Load() != 1 {
				t.Fatalf("valid message projection failed: error=%v response=%v attempts=%d", err, response, attempts.Load())
			}
			want := `{"messages":` + fixture.messages + `,"model":"gemini-3.1-pro-preview","stream":false}`
			if got := <-requests; got != want {
				t.Fatalf("message wire differs: got=%s want=%s", got, want)
			}
		})
	}
}
