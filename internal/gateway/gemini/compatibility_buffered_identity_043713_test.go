package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestProdex043713GeminiAPIKeyBufferedAlwaysReplacesProviderID(t *testing.T) {
	for _, tc := range []struct{ name, sourceID string }{
		{"anonymous", "resp_gemini_opaque"},
		{"native upstream ID", "stale-nested-id"},
		{"trace ID", "synthetic-trace-id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"responseId": tc.sourceID, "modelVersion": "gemini-3.1-pro-preview", "created_at": float64(1),
				"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "synthetic-ok"}}}}},
			})
			translated, err := translateGeminiChatOrNativeResponse043711(&http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string(body))),
			}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer translated.Body.Close()
			var output map[string]any
			if err := json.NewDecoder(translated.Body).Decode(&output); err != nil {
				t.Fatal(err)
			}
			id, _ := output["id"].(string)
			if !strings.HasPrefix(id, "resp_deepseek_") || id == tc.sourceID {
				t.Fatalf("API-key response leaked provider identity %q: %#v", tc.sourceID, output)
			}
			parsed, err := uuid.Parse(strings.TrimPrefix(id, "resp_deepseek_"))
			if err != nil || parsed.Version() != 7 {
				t.Fatalf("non UUIDv7 identity: %q %v", id, err)
			}
			if _, exists := output["created_at"]; exists {
				t.Fatal("API-key buffered response leaked native timestamp")
			}
			if output["model"] != "gemini-3.1-pro-preview" || translated.Header.Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("native metadata lost during ID normalization: %#v headers=%#v", output, translated.Header)
			}
		})
	}
}
