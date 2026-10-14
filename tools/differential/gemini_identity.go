package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Prodex 0.437.11's Gemini API-key Super bridge generates a UUIDv7 using
// the shared deepseek-compatible response namespace for successful buffered
// native GenerateContent envelopes. There is no created_at field. The
// JSON comparator may ignore randomized *values* only after this invariant
// verifies their type, version and presence/absence.
func geminiBufferedIdentityInvariant043711(root map[string]any) error {
	if _, found := root["created_at"]; found {
		return errors.New("Gemini buffered chat-compatible response invented created_at")
	}
	id, ok := root["id"].(string)
	if !ok || !strings.HasPrefix(id, "resp_deepseek_") {
		return fmt.Errorf("Gemini buffered chat-compatible response ID namespace differs: %q", id)
	}
	parsed, err := uuid.Parse(strings.TrimPrefix(id, "resp_deepseek_"))
	if err != nil || parsed.Version() != 7 {
		return fmt.Errorf("Gemini buffered chat-compatible response ID is not UUIDv7: %q", id)
	}
	return nil
}
