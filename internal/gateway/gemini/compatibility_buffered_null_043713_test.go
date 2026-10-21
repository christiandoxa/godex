package gemini

import (
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Malformed untrusted upstream JSON may not panic while assigning a new
// client-visible API-key Responses identity. Only a JSON object is valid.
func TestProdex043713GeminiAPIKeyRejectsNonObjectBufferedEnvelope(t *testing.T) {
	for _, payload := range []string{"null", `[]`, `"invalid"`, ``} {
		t.Run(payload, func(t *testing.T) {
			_, err := normalizeGeminiAPIKeyBufferedIdentity043711(&proxymodel.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(payload)),
			}, nil)
			if err == nil {
				t.Fatal("non-object Gemini API-key upstream body accepted")
			}
		})
	}
}
