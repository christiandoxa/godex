package routing

import (
	"testing"
	"time"
)

func TestRetryBackoffValidatesAndExpires(t *testing.T) {
	now := time.Unix(100, 0)
	backoff := RetryBackoff{AccountID: "account-a", UntilUnix: now.Add(20 * time.Second).Unix()}
	if err := backoff.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := backoff.Remaining(now); got != 20*time.Second {
		t.Fatalf("remaining backoff = %s, want 20s", got)
	}
	if got := backoff.Remaining(now.Add(20 * time.Second)); got != 0 {
		t.Fatalf("expired backoff = %s, want 0", got)
	}
	if err := (RetryBackoff{AccountID: "bad account", UntilUnix: 200}).Validate(); err == nil {
		t.Fatal("invalid account ID accepted")
	}
}
