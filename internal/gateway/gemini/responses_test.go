package gemini

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGeminiTranslatedResponseMergesRequestAndProviderMetadata(t *testing.T) {
	response, err := translateResponse(&http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"chat_1","model":"gemini-3.5-flash","choices":[{"message":{"content":"ok","reasoning_content":"thinking"},"finish_reason":"stop"}]}`)),
	}, map[string]any{
		"request_id": "req-1", "client_metadata": map[string]any{"client": "codex"},
		"gemini": map[string]any{"prompt_cache_key": "cache-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"request_id":"req-1"`, `"client_metadata":{"client":"codex"}`,
		`"prompt_cache_key":"cache-1"`, `"reasoning_content":"thinking"`, `"finish_reason":"stop"`,
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("translated response missing %s: %s", want, body)
		}
	}
}
