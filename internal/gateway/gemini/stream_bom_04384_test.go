package gemini

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384GeminiProviderPayloadBOMStaysInvalid(t *testing.T) {
	for _, native := range []bool{false, true} {
		chunk := `{"choices":[{"delta":{"content":"bom-control"}}]}`
		if native {
			chunk = `{"candidates":[{"content":{"parts":[{"text":"bom-control"}]}}]}`
		}
		for _, prefix := range []string{"", "\xef\xbb\xbf", "\xef\xbb\xbf\xef\xbb\xbf"} {
			upstream := &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader("data: " + prefix + chunk + "\n\ndata: [DONE]\n\n")),
			}
			var body io.ReadCloser
			if native {
				body = translateGeminiNativeStream(upstream, nil).Body
			} else {
				body = translateGeminiChatCompatibleStream043711(context.Background(), upstream, nil, nil).Body
			}
			output, err := io.ReadAll(body)
			body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if prefix == "" {
				if !strings.Contains(string(output), "bom-control") || !strings.Contains(string(output), "event: response.completed") {
					t.Fatalf("normal stream rejected: native=%v output=%s", native, output)
				}
				continue
			}
			if strings.Count(string(output), "event: ") != 1 ||
				!strings.Contains(string(output), `"message":"incomplete JSON segment"`) ||
				strings.Contains(string(output), "bom-control") {
				t.Errorf("provider payload BOM accepted: native=%v prefix=%q output=%s", native, prefix, output)
			}
		}
	}
}
