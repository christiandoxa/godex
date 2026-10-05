package deepseek

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func assertDeepSeekUUIDv7(t *testing.T, value any, prefix string) string {
	t.Helper()
	text, ok := value.(string)
	if !ok || !strings.HasPrefix(text, prefix) {
		t.Fatalf("fallback ID = %#v, want prefix %q", value, prefix)
	}
	id, err := uuid.Parse(strings.TrimPrefix(text, prefix))
	if err != nil || id.Version() != 7 {
		t.Fatalf("fallback ID = %q, want UUIDv7: %v", text, err)
	}
	return text
}

func TestProdex04355DeepSeekFallbackIDsAreUniqueUUIDv7(t *testing.T) {
	firstResponse := deepSeekResponseFallbackID()
	secondResponse := deepSeekResponseFallbackID()
	firstCall := deepSeekCallFallbackID()
	secondCall := deepSeekCallFallbackID()
	assertDeepSeekUUIDv7(t, firstResponse, "resp_deepseek_")
	assertDeepSeekUUIDv7(t, secondResponse, "resp_deepseek_")
	assertDeepSeekUUIDv7(t, firstCall, "call_deepseek_")
	assertDeepSeekUUIDv7(t, secondCall, "call_deepseek_")
	if firstResponse == secondResponse || firstCall == secondCall {
		t.Fatalf("fallback IDs were reused: %q/%q %q/%q", firstResponse, secondResponse, firstCall, secondCall)
	}
}
