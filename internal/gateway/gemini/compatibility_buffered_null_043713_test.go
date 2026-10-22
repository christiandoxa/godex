package gemini

import "testing"

// Malformed untrusted upstream JSON may not panic while assigning a new
// client-visible API-key Responses identity. Only a JSON object is valid.
func TestProdex043713GeminiAPIKeyRejectsNonObjectBufferedEnvelope(t *testing.T) {
	for _, payload := range []string{"null", `[]`, `"invalid"`, ``} {
		t.Run(payload, func(t *testing.T) {
			object, _ := geminiSerdeResponseObject04384([]byte(payload))
			err := normalizeGeminiAPIKeyBufferedValue04384(object, nil)
			if err == nil {
				t.Fatal("non-object Gemini API-key upstream body accepted")
			}
		})
	}
}
