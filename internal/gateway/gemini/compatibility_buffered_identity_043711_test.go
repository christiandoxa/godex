package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/google/uuid"
)

// A pinned 0.437.11 official executable comparison shows that successful
// native candidates forwarded over the Gemini API-key *chat-compatible*
// bridge get a resp_deepseek_ UUIDv7, with no created_at field. The
// separate native Gemini transport must not inherit this adapter change.
func TestProdex043711GeminiAPIKeyBufferedResponseIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/openai/chat/completions" {
			t.Errorf("unexpected Gemini API-key upstream route: %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview","usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`)
	}))
	defer server.Close()
	pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{
		{ID: "key-a", Secret: "synthetic-credential"},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	result, err := pool.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses",
		Body: []byte(`{"model":"gemini-3.1-pro-preview","input":"synthetic","stream":false}`),
	}, proxymodel.Account{ID: "key-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(result.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || payload["model"] != "gemini-3.1-pro-preview" {
		t.Fatalf("Gemini API-key native response shape regressed: %+v", payload)
	}
	if _, exists := payload["created_at"]; exists {
		t.Fatalf("tagged API-key buffered Gemini response must not invent created_at: %+v", payload)
	}
	id, ok := payload["id"].(string)
	if !ok || !strings.HasPrefix(id, "resp_deepseek_") {
		t.Fatalf("tagged Gemini API-key buffered response has wrong ID namespace: %q", id)
	}
	parsed, err := uuid.Parse(strings.TrimPrefix(id, "resp_deepseek_"))
	if err != nil || parsed.Version() != 7 {
		t.Fatalf("Gemini API-key buffered ID is not UUIDv7: %q, %v", id, err)
	}
}
