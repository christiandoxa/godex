package gemini

import (
	"errors"
	"github.com/google/uuid"
)

// The tagged API-key bridge generates a UUIDv7 only when the original
// upstream id is not a string. Otherwise it keeps the provider transform's
// responseId/traceId identity; the original id is never copied as an alias.
func normalizeGeminiAPIKeyBufferedValue04384(object, source map[string]any) error {
	if object == nil {
		return errors.New("invalid Gemini API-key translated response")
	}
	delete(object, "created_at")
	if _, hasStringID := source["id"].(string); !hasStringID {
		generated, err := uuid.NewV7()
		if err != nil {
			return errors.New("generate tagged Gemini Responses identity")
		}
		object["id"] = "resp_deepseek_" + generated.String()
	}
	return nil
}
