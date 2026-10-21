package routing

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Tagged runtime_retry_after_from_headers is the numeric-only compiled Mojo
// header parser, capped at 300 seconds. It does not parse HTTP-date values.
// The earlier generic Go helper accepted HTTP-date and up to 24-hour delays
// for a native quota error, incorrectly extending the account quarantine.
func TestProdex04383HTTP429NativeQuotaRetryAfterUsesMojoNumericCap(t *testing.T) {
	now := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, headerValue string
		want              time.Duration
	}{
		{"numeric-300", "300", 5 * time.Minute},
		{"numeric-over-cap", "3600", 5 * time.Minute},
		{"numeric-very-large", "9999999999", 5 * time.Minute},
		{"future-date", "Wed, 22 Oct 2031 10:00:00 GMT", 5 * time.Minute},
		{"valid-date-for-Go", "Wed, 21 Oct 2026 13:00:00 GMT", 5 * time.Minute},
		{"invalid-date", "no-date", 5 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := &Router{now: func() time.Time { return now }, maxInspect: 64 << 10}
			response := &proxymodel.Response{StatusCode: http.StatusTooManyRequests,
				Header: http.Header{"Retry-After": {tc.headerValue}},
				Body:   io.NopCloser(strings.NewReader(`{"error":{"code":"insufficient_quota"}}`)),
			}
			outcome, pending, err := router.classify(response, "openai")
			if err != nil {
				t.Fatal(err)
			}
			if pending != nil {
				defer pending.close()
			}
			if outcome.kind != responseRetry || !outcome.quota || outcome.quarantine != tc.want {
				t.Fatalf("native HTTP429 quota header=%q outcome=%+v want cooldown=%s", tc.headerValue, outcome, tc.want)
			}
		})
	}
}

func TestProdex04383NativeRetryHeaderUsesAllValidNumericCandidates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []string
		want   time.Duration
	}{
		{"numeric-max", []string{"90", "240"}, 240 * time.Second},
		{"numeric-cap", []string{"90", "3600"}, 5 * time.Minute},
		{"date-not-candidate", []string{"Wed, 21 Oct 2026 13:00:00 GMT", "180"}, 180 * time.Second},
		{"only-date", []string{"Wed, 21 Oct 2026 13:00:00 GMT"}, 0},
		{"invalid-after-valid", []string{"100", "invalid"}, 100 * time.Second},
		{"zero-only", []string{"0"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{"Retry-After": tc.values}
			if got := nativeRetryAfterHeaders04383(headers); got != tc.want {
				t.Fatalf("numeric Mojo advice=%s want=%s", got, tc.want)
			}
		})
	}
}

func TestProdex04383NativeSSEQuotaRejectsFutureHTTPDateRetry(t *testing.T) {
	now := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}}}`)
	for _, tc := range []struct {
		name, raw string
		want      time.Duration
	}{
		{"date-ignored", "Wed, 21 Oct 2026 13:00:00 GMT", 5 * time.Minute},
		{"numeric-capped", "3600", 5 * time.Minute},
		{"numeric-valid", "240", 5 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{"Retry-After": {tc.raw}}
			outcome, wait := streamOutcome(payload, headers, now, "openai")
			if wait || outcome.kind != responseRetry || !outcome.quota || outcome.quarantine != tc.want {
				t.Fatalf("quota SSE Retry-After %q outcome=%+v wait=%t", tc.raw, outcome, wait)
			}
		})
	}
}

func TestProdex04383NativeHeaderChangeLeavesExternalDateParserUnchanged(t *testing.T) {
	now := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	header := http.Header{"Retry-After": {"Wed, 21 Oct 2026 13:00:00 GMT"}}
	if got := retryAfter(header, now); got != 3*time.Hour {
		t.Fatalf("external provider HTTP date advice unexpectedly changed=%s", got)
	}
}
