package routing

import (
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	"testing"
	"time"
)

// Tagged 0.438.3 quota_retry_time_candidate normalizes two ASCII-separated
// clock tokens before chrono 0.4.45 parses "%I:%M %p". Chrono accepts
// ASCII case-insensitive AM/PM and 1- or 2-digit minutes, but never hour 0.
func TestProdex04383QuotaRetryClockMatchesPinnedChrono(t *testing.T) {
	now := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, clock          string
		wantHour, wantMinute int
		valid                bool
	}{
		{"canonical", "5:08 PM", 17, 8, true},
		{"lowercase", "5:08 pm", 17, 8, true},
		{"mixed-case", "5:08 pM", 17, 8, true},
		{"upper-lower", "5:08 Pm", 17, 8, true},
		{"short-minute", "1:8 PM", 13, 8, true},
		{"double-space-normalized", "1:8  PM", 13, 8, true},
		{"two-digit-hour", "05:08 PM", 17, 8, true},
		{"twelve-am", "12:04 am", 0, 4, true},
		{"twelve-pm", "12:04 PM", 12, 4, true},
		{"zero-hour", "0:08 PM", 0, 0, false},
		{"double-zero-hour", "00:08 AM", 0, 0, false},
		{"too-large-hour", "13:08 PM", 0, 0, false},
		{"too-many-minute-digits", "1:008 PM", 0, 0, false},
		{"invalid-minute", "1:60 PM", 0, 0, false},
		{"unrecognized-ampm", "1:08 P.M.", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at, valid := quotaResetAtFromText("Synthetic quota. Try again at "+tc.clock+".", now)
			if valid != tc.valid {
				t.Fatalf("chrono clock %q valid=%t want=%t", tc.clock, valid, tc.valid)
			}
			if !valid {
				return
			}
			expected := time.Date(2026, time.October, 21, tc.wantHour, tc.wantMinute, 0, 0, time.UTC)
			if !expected.After(now) {
				expected = expected.AddDate(0, 0, 1)
			}
			if at != expected.Unix() {
				t.Fatalf("chrono clock %q resolved at %d want %d", tc.clock, at, expected.Unix())
			}
		})
	}
}

func TestProdex04383QuotaRetryAbsoluteMatchesPinnedChrono(t *testing.T) {
	now := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, candidate string
		expect          string
		valid           bool
	}{
		{"canonical", "Mar 24th, 2026 2:04 AM", "2026-03-24 02:04", true},
		{"lowercase-month-ampm", "mar 24th, 2026 2:04 am", "2026-03-24 02:04", true},
		{"short-minute", "Mar 24th, 2026 2:4 AM", "2026-03-24 02:04", true},
		{"mixed-ampm", "Mar 24th, 2026 2:4 aM", "2026-03-24 02:04", true},
		{"month-all-uppercase", "MAR 24th, 2026 2:04 PM", "2026-03-24 14:04", true},
		{"short-year-26", "Mar 24th, 26 2:04 AM", "0026-03-24 02:04", true},
		{"early-year-0026", "Mar 24th, 0026 2:04 AM", "0026-03-24 02:04", true},
		{"zero-hour", "Mar 24th, 2026 0:04 AM", "", false},
		{"feb29-nonleap", "Feb 29th, 2026 1:8 PM", "", false},
		{"full-month-rejected", "March 24th, 2026 2:04 AM", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at, valid := quotaResetAtFromText("quota try again at "+tc.candidate+".", now)
			if valid != tc.valid {
				t.Fatalf("chrono datetime %q accepted=%t want=%t", tc.candidate, valid, tc.valid)
			}
			if !valid {
				return
			}
			expected, err := time.ParseInLocation("2006-01-02 15:04", tc.expect, time.UTC)
			if err != nil {
				t.Fatal(err)
			}
			if at != expected.Unix() {
				t.Fatalf("chrono datetime %q at=%d want=%d", tc.candidate, at, expected.Unix())
			}
		})
	}
}

func TestProdex04383QuotaChronoClockChangesActualQuarantine(t *testing.T) {
	now := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	router := &Router{now: func() time.Time { return now }}
	for _, tc := range []struct {
		name, body string
		want       time.Duration
	}{
		{"lowercase-am-pm", "Try again at 5:08 pm.", 7*time.Hour + 8*time.Minute},
		{"short-minute", "Try again at 1:8 PM.", 3*time.Hour + 8*time.Minute},
		{"zero-hour-rejected", "Try again at 0:08 PM.", prodex04357QuotaQuarantineFallback},
		{"mixed-case", "Try again at 5:08 pM.", 7*time.Hour + 8*time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset := quotaResetAtFromMessage([]byte(tc.body), now)
			got := router.quotaQuarantineDuration("account-a", quotamodel.Selection{}, reset)
			if got != tc.want {
				t.Fatalf("quota chrono %q gave=%s want=%s", tc.name, got, tc.want)
			}
		})
	}
}
