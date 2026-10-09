package deepseek

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// Prodex 0.437.0 emits one terminal provider_stream_error when a syntactically
// valid but incomplete DeepSeek SSE feed reaches EOF before [DONE].
func TestProdex04370DeepSeekPrematureStreamEOFUsesCanonicalError(t *testing.T) {
	upstream := "event: response.failed\r\ndata: {\"type\":\"response.failed\"}\r\n\r\n"
	stream := deepSeekChatSSE(io.NopCloser(strings.NewReader(upstream)))
	defer stream.Close()
	payload, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if strings.Count(text, "event: response.failed") != 1 ||
		!strings.Contains(text, "\"code\":\"provider_stream_error\"") ||
		!strings.Contains(text, "\"message\":\"unexpected end of stream\"") ||
		strings.Contains(text, "event: response.completed") {
		t.Fatalf("premature provider stream EOF contract = %q", text)
	}
}

// Prodex has entered the translated DeepSeek SSE response writer before it
// surfaces embedded provider errors. Those errors are client-visible, not a
// second precommit retry opportunity for a different API key.
func TestProdex04370DeepSeekTranslatedSSECommitsBeforeProviderError(t *testing.T) {
	wire := "data: {\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"Please try again in 1s.\",\"headers\":{\"Retry-After\":\"5\"}}}\r\n\r\n"
	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(wire)),
	}
	response, err := translateResponseWithConversation(upstream, nil, newDeepSeekConversationStore(), nil, 123)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if !response.FirstEventCommitted {
		t.Fatal("translated DeepSeek stream left an already selected writer retryable")
	}
	got, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "\"code\":\"rate_limit_exceeded\"") ||
		!strings.Contains(string(got), "\"message\":\"Please try again in 1s.\"") {
		t.Fatalf("provider error lost while marking commit: %q", got)
	}
}
