package routing

import (
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	"testing"
	"time"
)

// Tagged quota_reset_at_from_message delegates finding `try again at ` to
// the Mojo byte-level ASCII-casefold matcher. Unicode U+0130 becomes an
// ASCII 'i' under Go strings.ToLower, but must NOT make a false marker.
// Conversely, Unicode text before a legitimate ASCII marker must not change
// its original byte offset and truncate the parsed time suffix.
func TestProdex04383QuotaRetryMarkerUsesASCIIByteOffset(t *testing.T) {
	now := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	later := time.Date(2026, time.October, 21, 17, 8, 0, 0, time.UTC).Unix()
	for _, tc := range []struct {
		name, message string
		want          int64
	}{
		{"canonical", "You've hit your quota; Try again at 5:08 PM.", later},
		{"uppercase", "TRY AGAIN AT 5:08 PM.", later},
		{"non-ascii-prefix", "İİİTry again at 5:08 PM.", later},
		{"non-ascii-after-punctuation", "quota:İİİ Try again at 5:08 PM.", later},
		{"unicode-confusable-marker", "Try agaİn at 5:08 PM.", 0},
		{"unicode-confusable-upper", "TRY AGAİN AT 5:08 PM.", 0},
		{"only-prefix", "İİİ quota exceeded", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := quotaResetAtFromText(tc.message, now)
			wantOK := tc.want != 0
			if ok != wantOK || got != tc.want {
				t.Fatalf("quota text %q parsed=%d exists=%t want=%d exists=%t", tc.message, got, ok, tc.want, wantOK)
			}
		})
	}
}

func TestProdex04383UnicodeQuotaMarkerCannotForgeLongCooldown(t *testing.T) {
	now := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, message string
		want          time.Duration
	}{
		{"normal", "Try again at 5:08 PM.", 7*time.Hour + 8*time.Minute},
		{"confusable", "Try agaİn at 5:08 PM.", 0},
		{"unicode-prefix", "İİİTry again at 5:08 PM.", 7*time.Hour + 8*time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetAt := quotaResetAtFromMessage([]byte(tc.message), now)
			var delay time.Duration
			if resetAt > 0 {
				delay = time.Unix(resetAt, 0).Sub(now)
			}
			if delay != tc.want {
				t.Fatalf("Unicode marker forged quota cooldown=%s want=%s", delay, tc.want)
			}
		})
	}
}

// A forged Unicode marker must not impose a multi-hour quota block. A real
// ASCII marker after a variable-width Unicode prefix still means that the
// quota account can resume at the source-provided local clock timestamp.
func TestProdex04383UnicodeQuotaMarkerChangesActualQuotaQuarantine(t *testing.T) {
	now := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	router := &Router{now: func() time.Time { return now }}
	for _, tc := range []struct {
		name, message string
		want          time.Duration
	}{
		{"ASCII-control", "Try again at 5:08 PM.", 7*time.Hour + 8*time.Minute},
		{"Unicode-forged", "Try agaİn at 5:08 PM.", prodex04357QuotaQuarantineFallback},
		{"Unicode-before-ASCII", "İİİTry again at 5:08 PM.", 7*time.Hour + 8*time.Minute},
		{"Unicode-forged-then-valid", "Try agaİn at 5:08 PM. Try again at 4:08 PM.", 6*time.Hour + 8*time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset := quotaResetAtFromMessage([]byte(tc.message), now)
			got := router.quotaQuarantineDuration("account-a", quotamodel.Selection{}, reset)
			if got != tc.want {
				t.Fatalf("Unicode quota reset changed quarantine: got=%s want=%s reset=%d", got, tc.want, reset)
			}
		})
	}
}
