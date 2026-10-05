package routing

import (
	"testing"
	"time"
)

func TestTransportBackoffValidatesAndExpires(t *testing.T) {
	now := time.Unix(100, 0)
	backoff := TransportBackoff{AccountID: "account-a", Route: "responses", UntilUnix: 130}
	if err := backoff.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := backoff.Remaining(now); got != 30*time.Second {
		t.Fatalf("remaining transport backoff = %s, want 30s", got)
	}
	if got := backoff.Remaining(time.Unix(130, 0)); got != 0 {
		t.Fatalf("expired transport backoff = %s, want 0", got)
	}
	for _, invalid := range []TransportBackoff{
		{AccountID: "bad account", Route: "responses", UntilUnix: 130},
		{AccountID: "account-a", Route: "unknown", UntilUnix: 130},
		{AccountID: "account-a", Route: "responses", UntilUnix: 0},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid transport backoff accepted: %+v", invalid)
		}
	}
}
