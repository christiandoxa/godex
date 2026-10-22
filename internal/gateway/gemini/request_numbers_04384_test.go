package gemini

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiChatRequestPreservesSerdeToolNumbers(t *testing.T) {
	for _, fixture := range []struct{ name, input, want string }{
		{"safe_integer", "9007199254740991", "9007199254740991"},
		{"large_integer", "9007199254740993", "9007199254740993"},
		{"unsigned_limit", "18446744073709551615", "18446744073709551615"},
		{"signed_limit", "-9223372036854775808", "-9223372036854775808"},
		{"negative_large_integer", "-9007199254740993", "-9007199254740993"},
		{"exponent", "1e+03", "1000.0"},
		{"negative_zero", "-0", "-0.0"},
		{"fraction", "0.123456789012345678901234567890", "0.12345678901234568"},
		{"unsigned_overflow", "18446744073709551616", "1.8446744073709552e+19"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream_%t", fixture.name, stream), func(t *testing.T) {
				var mu sync.Mutex
				var captured string
				var attempts int
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
					if err != nil {
						t.Error(err)
					}
					mu.Lock()
					attempts++
					captured = string(body)
					mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]}}]}`)
				}))
				defer server.Close()
				pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{{ID: "synthetic", Secret: "synthetic-only"}}, server.Client())
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				body := fmt.Sprintf(`{"model":"gemini-3.1-pro-preview","input":"same request","stream":%t,"tools":[{"type":"function","function":{"name":"numeric_fixture","parameters":{"type":"object","properties":{"value":{"type":"number","enum":[%s]}}}}}]}`, stream, fixture.input)
				reply, err := pool.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost,
					Path: "/backend-api/godex/responses", Body: []byte(body)}, proxymodel.Account{ID: "synthetic"})
				if err != nil {
					t.Fatal(err)
				}
				defer reply.Body.Close()
				mu.Lock()
				defer mu.Unlock()
				if reply.StatusCode != 200 || attempts != 1 || !strings.Contains(captured, `"enum":[`+fixture.want+`]`) {
					t.Fatalf("tool number differs from locked serde: status=%d attempts=%d body=%s", reply.StatusCode, attempts, captured)
				}
			})
		}
	}
}

func TestProdex04384GeminiChatRequestDecodersRequireCompleteObject(t *testing.T) {
	for _, normalize := range []func([]byte) ([]byte, error){
		func(body []byte) ([]byte, error) {
			value, _, err := normalizeGeminiChatInput043711(body)
			return value, err
		},
		filterGeminiOrphanToolMessages043711,
		func(body []byte) ([]byte, error) { return geminiChatRequestOptions04384(body, nil) },
	} {
		for _, body := range []string{`{"stream":true,"messages":[]} {"extra":true}`, `[{"stream":true}]`} {
			if _, err := normalize([]byte(body)); err == nil {
				t.Fatal("chat request normalization accepted trailing JSON or an array root")
			}
		}
	}
}
