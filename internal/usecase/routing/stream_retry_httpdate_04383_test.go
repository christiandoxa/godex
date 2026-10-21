package routing

import (
	"encoding/json"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"net/http"
	"testing"
	"time"
)

// Prodex pins httpdate 1.0.3, whose decoder requires the weekday to match
// the actual calendar date and fixed-width HTTP fields. net/http.ParseTime
// accepts incorrect weekdays and some noncanonical field widths.
func TestProdex04383RetryAfterDateRequiresValidHttpdateCalendar(t *testing.T) {
	now := time.Date(2026, time.October, 21, 9, 53, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, value string
		valid       bool
		want        time.Duration
	}{
		{"canonical", "Wed, 21 Oct 2026 09:54:00 GMT", true, time.Minute},
		{"canonical-trailing-space", "Wed, 21 Oct 2026 09:54:00 GMT ", true, time.Minute},
		{"wrong-weekday", "Mon, 21 Oct 2026 09:54:00 GMT", false, 0},
		{"noncanonical-hour", "Wed, 21 Oct 2026 9:54:00 GMT", false, 0},
		{"rfc850-canonical", "Wednesday, 21-Oct-26 09:54:00 GMT", true, time.Minute},
		{"rfc850-wrong-weekday", "Tuesday, 21-Oct-26 09:54:00 GMT", false, 0},
		{"rfc850-year69", "Monday, 21-Oct-69 09:54:00 GMT", true, 5 * time.Minute},
		{"rfc850-year69-wrong-day", "Tuesday, 21-Oct-69 09:54:00 GMT", false, 0},
		{"asctime-canonical", "Wed Oct 21 09:54:00 2026", true, time.Minute},
		{"asctime-wrong-day", "Mon Oct 21 09:54:00 2026", false, 0},
		{"pre-epoch", "Wed, 31 Dec 1969 23:59:59 GMT", false, 0},
		{"valid-past", "Sun, 06 Nov 1994 08:49:37 GMT", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, valid := parseStreamRetryHeader(tc.value, now)
			if valid != tc.valid || (valid && got != tc.want) {
				t.Fatalf("date %q delay=%s valid=%t wantDelay=%s wantValid=%t", tc.value, got, valid, tc.want, tc.valid)
			}
		})
	}
}

// A syntactically accepted but calendar-invalid HTTP-date must not override
// the tagged structured error-message fallback on the precommit adapter.
func TestProdex04383HTTPDateValidityControlsRealPrecommitCooldown(t *testing.T) {
	now := time.Date(2026, time.October, 21, 9, 53, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, date string
		want       time.Duration
	}{
		{"valid-imf", "Wed, 21 Oct 2026 09:54:00 GMT", time.Minute},
		{"weekday-mismatch", "Mon, 21 Oct 2026 09:54:00 GMT", 12 * time.Second},
		{"noncanonical-hour", "Wed, 21 Oct 2026 9:54:00 GMT", 12 * time.Second},
		{"rfc850-year69", "Monday, 21-Oct-69 09:54:00 GMT", 5 * time.Minute},
		{"rfc850-wrong-weekday", "Tuesday, 21-Oct-69 09:54:00 GMT", 12 * time.Second},
		{"pre-epoch", "Wed, 31 Dec 1969 23:59:59 GMT", 12 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Please try again in 12s.","headers":{"Retry-After":` + quotedHTTPDate04383(tc.date) + `}}}}`
			response := &proxymodel.Response{StatusCode: http.StatusOK, PrecommitFailure: &proxymodel.PrecommitFailure{
				Code: "rate_limit_exceeded", RetryAdviceJSON: []byte(body)}}
			router := &Router{now: func() time.Time { return now }}
			outcome, _, err := router.classifyPrecommitFailure(response, &pendingResponse{response: response})
			if err != nil {
				t.Fatal(err)
			}
			if outcome.kind != responseRetry || !outcome.explicitRetryAdvice || outcome.quarantine != tc.want {
				t.Fatalf("invalid HTTP-date overrode precommit delay: %+v want=%s", outcome, tc.want)
			}
		})
	}
}

func quotedHTTPDate04383(text string) string { bytes, _ := json.Marshal(text); return string(bytes) }
