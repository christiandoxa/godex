package gemini

import (
	"errors"
	"strings"
)

const invalidGeminiUserID04384 = "Gemini OpenAI-compatible user_id must use only letters, numbers, underscores, or dashes and be at most 512 bytes"

func normalizeGeminiUserID04384(request map[string]any) error {
	for _, key := range []string{"user_id", "user", "safety_identifier"} {
		value, present := request[key]
		if !present {
			continue
		}
		text, valid := value.(string)
		if !valid {
			return errors.New("Gemini OpenAI-compatible user_id must be a string")
		}
		text = strings.TrimSpace(text)
		if len(text) > 512 {
			return errors.New(invalidGeminiUserID04384)
		}
		for _, char := range text {
			if char != '_' && char != '-' && !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') {
				return errors.New(invalidGeminiUserID04384)
			}
		}
		for _, alias := range []string{"user_id", "user", "safety_identifier"} {
			delete(request, alias)
		}
		if text != "" {
			// The shared Chat converter consumes user; the Gemini wire uses user_id.
			request["user"] = text
		}
		return nil
	}
	return nil
}
