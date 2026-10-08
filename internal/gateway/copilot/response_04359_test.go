package copilot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Exact 0.435.9 Copilot response-ID policy preserves JSON string decoding
// and Unicode-trim semantics. Godex uses a lossless upstream forwarding
// boundary instead of a provider-core extraction/rewrite stage: preserve all
// original response bytes so downstream affinity can interpret the same
// nested, top-level, blank and Unicode response-ID shapes.
func TestProdex04359CopilotResponseShapesRemainLossless(t *testing.T) {
	fixtures := []string{
		`{"response":{"id":"\u2003"},"id":"resp_top"}`,
		`{"response":{"id":" 🦀 resp 🦀 "}}`,
		`{"id":"resp_top"}`,
		`{"response_id":"resp_event"}`,
	}
	responseIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if responseIndex >= len(fixtures) {
			http.Error(w, "too many requests", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixtures[responseIndex])
		responseIndex++
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, RuntimeAuth{apiKey: "synthetic-test-credential"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	for i, want := range fixtures {
		reply, err := transport.Execute(context.Background(), proxymodel.Request{
			Method: http.MethodPost, Path: "/backend-api/godex/responses",
			Body: []byte(`{"model":"codex","input":"hello"}`),
		}, proxymodel.Account{})
		if err != nil {
			t.Fatalf("fixture %d: %v", i, err)
		}
		got, readErr := io.ReadAll(reply.Body)
		closeErr := reply.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("fixture %d read=%v close=%v", i, readErr, closeErr)
		}
		if reply.StatusCode != http.StatusOK || string(got) != want {
			t.Fatalf("fixture %d: status=%d body=%q; want %q", i, reply.StatusCode, got, want)
		}
	}
}
