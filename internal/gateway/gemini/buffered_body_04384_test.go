package gemini

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384GeminiBufferedBodyBoundary(t *testing.T) {
	const limit = 4 << 20
	const prefix, suffix = `{"candidates":[{"content":{"parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview","padding":"`, `"}`
	for _, fixture := range []struct {
		name string
		size int
	}{{"under", limit - 1}, {"limit", limit}, {"over", limit + 1}} {
		t.Run(fixture.name, func(t *testing.T) {
			body := prefix + strings.Repeat(" ", fixture.size-len(prefix)-len(suffix)) + suffix
			response, err := translateGeminiChatOrNativeResponse043711(&http.Response{StatusCode: 200,
				Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))})
			if fixture.size > limit {
				if err == nil || response != nil {
					t.Fatal("Gemini body above 4 MiB reached translation")
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("admitted Gemini body was rejected: %v", err)
			}
			defer response.Body.Close()
			output, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != 200 || !strings.Contains(string(output), "synthetic-ok") {
				t.Fatalf("admitted Gemini body lost native output: %v", err)
			}
		})
	}
}
