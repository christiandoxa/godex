package routing

import (
	"testing"
	"time"
)

func TestProdex043714StructuredRetryAfterOverflowFallsBackToMessage(t *testing.T) {
	const event = `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Please try again in 12s.","headers":{"Retry-After":"18446744073709551616"}}}}`

	got, ok := structuredStreamRetryAdvice([]byte(event), time.Unix(1_000_000_000, 0))
	if !ok || got != 12*time.Second {
		t.Fatalf("overflow Retry-After advice = %s/%t, want 12s/true", got, ok)
	}
}
