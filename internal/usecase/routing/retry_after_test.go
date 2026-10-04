package routing

import (
	"net/http"
	"testing"
	"time"
)

func TestRateLimitCooldownUsesTaggedRetryHintAndDefault(t *testing.T) {
	for _, test := range []struct {
		name   string
		header string
		body   string
		want   time.Duration
	}{
		{name: "default raises a shorter message hint", body: `{"error":{"message":"Please try again in 1s."}}`, want: 20 * time.Second},
		{name: "longer message hint wins", body: `{"error":{"message":"Please try again in 30s."}}`, want: 30 * time.Second},
		{name: "header hint", header: "45", want: 45 * time.Second},
		{name: "header maximum", header: "301", want: 5 * time.Minute},
		{name: "HTTP date is not a tagged retry hint", header: "Wed, 21 Oct 2015 07:28:00 GMT", want: 20 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := make(http.Header)
			if test.header != "" {
				headers.Set("Retry-After", test.header)
			}
			if got := rateLimitCooldown(headers, []byte(test.body)); got != test.want {
				t.Fatalf("rate-limit cooldown = %s, want %s", got, test.want)
			}
		})
	}
}
