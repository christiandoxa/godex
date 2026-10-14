package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// The pinned 0.437.11 Prodex executable returns a UTF-8 charset on every
// translated buffered Gemini API-key Responses envelope, even when the
// upstream's Content-Type is plain application/json. Non-2xx provider
// passthrough and the separate native transport retain their own policy.
func TestProdex043711GeminiAPIKeyBufferedContentTypeIncludesUTF8Charset(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"native-candidates", `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview"}`},
		{"chat-choices-invalid", `{"id":"chatcmpl-fixture","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ignored"},"finish_reason":"stop"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{{
				ID: "key-a", Secret: "synthetic-local-credential",
			}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			response, err := pool.Execute(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Path: "/backend-api/prodex/responses",
				Body: []byte(`{"model":"gemini-3.1-pro-preview","input":"synthetic","stream":false}`),
			}, proxymodel.Account{ID: "key-a"})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if got := response.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Fatalf("buffered Gemini API-key Content-Type=%q, want tagged 0.437.11 UTF-8 response", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["object"] != "response" {
				t.Fatalf("translated Gemini response no longer has Responses envelope: %+v", payload)
			}
		})
	}
}
