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

func TestProdex04384GeminiStopAdmissionAndAliases(t *testing.T) {
	const stops16 = `["s01","s02","s03","s04","s05","s06","s07","s08","s09","s10","s11","s12","s13","s14","s15","s16"]`
	for _, fixture := range []struct{ name, fields, want string }{
		{"string", `"stop":"STOP"`, `"STOP"`},
		{"empty_string", `"stop":""`, `""`},
		{"empty_array", `"stop":[]`, `[]`},
		{"snake", `"stop_sequences":[" STOP ",""]`, `[" STOP ",""]`},
		{"camel", `"stopSequences":"CAMEL"`, `"CAMEL"`},
		{"precedence", `"stop":"PRIMARY","stop_sequences":null,"stopSequences":false`, `"PRIMARY"`},
		{"alias_precedence", `"stop_sequences":"SECOND","stopSequences":"THIRD"`, `"SECOND"`},
		{"shadow_null", `"stop":null,"stop_sequences":"SECOND"`, ""},
		{"null", `"stop":null`, ""},
		{"boolean", `"stop":false`, ""},
		{"array_number", `"stop":["STOP",1]`, ""},
		{"limit", `"stop":` + stops16, stops16},
		{"over_limit", `"stop":` + strings.TrimSuffix(stops16, "]") + `,"s17"]`, ""},
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
			if fixture.want == "" {
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" || attempts.Load() != 0 {
					t.Fatalf("invalid stop lost terminal no-attempt admission: error=%v attempts=%d", err, attempts.Load())
				}
				return
			}
			if err != nil || reply == nil || attempts.Load() != 1 {
				t.Fatalf("valid stop did not reach upstream once: error=%v attempts=%d", err, attempts.Load())
			}
			want := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stop":` + fixture.want + `,"stream":false}`
			if body := <-captured; body != want {
				t.Fatalf("stop request bytes = %s, want %s", body, want)
			}
			body, err := io.ReadAll(reply.Body)
			if err != nil || reply.StatusCode != 200 || !strings.Contains(string(body), "synthetic-ok") {
				t.Fatal("valid stop lost native response")
			}
		})
	}
}
