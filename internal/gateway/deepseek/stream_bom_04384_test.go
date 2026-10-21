package deepseek

import (
	"io"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekProviderPayloadBOMIsNotJSONWhitespace(t *testing.T) {
	const chunk = `{"id":"chat-bom","created":1,"choices":[{"delta":{"content":"must-not-emit"}}]}`
	for _, prefix := range []string{"\xef\xbb\xbf", "\xef\xbb\xbf\xef\xbb\xbf"} {
		body := deepSeekChatSSE(io.NopCloser(strings.NewReader("data: " + prefix + chunk + "\n\ndata: [DONE]\n\n")))
		output, err := io.ReadAll(body)
		body.Close()
		if err != nil || !strings.Contains(string(output), `"message":"incomplete JSON segment"`) ||
			strings.Count(string(output), "event: ") != 1 ||
			strings.Contains(string(output), "must-not-emit") {
			t.Fatalf("provider data BOM was accepted as JSON whitespace: output=%s error=%v", output, err)
		}
	}
}
