package routing

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// After serde_json canonicalization, literal TAB/LF/CR inside JSON strings
// remain escaped (\\t, \\n, \\r). Mojo's quota_json_i64 trims unescaped
// ASCII whitespace only; Go must not decode such escapes then TrimSpace.
func TestProdex04383QuotaResetJSONNumericStringWhitespaceIsNotDecodedPadding(t *testing.T) {
	now := time.Unix(1_000, 0)
	for _, tc := range []struct {
		name, payload string
		want          int64
	}{
		{"ascii-spaces", `{"resets_at":"  2000  "}`, 2000},
		{"escaped-ascii-space", `{"resets_at":"\u00202000\u0020"}`, 2000},
		{"escaped-numeric-digits", `{"resets_at":"\u0032\u0030\u0030\u0030"}`, 2000},
		{"escaped-tab", `{"resets_at":"\t2000\t"}`, 0},
		{"escaped-newline", `{"resets_at":"\n2000\n"}`, 0},
		{"escaped-carriage-return", `{"resets_at":"\r2000\r"}`, 0},
		{"unicode-em-space", `{"resets_at":"\u20032000\u2003"}`, 0},
		{"unicode-nbsp", `{"resets_at":"\u00a02000\u00a0"}`, 0},
		{"header-valid-spaces", `{"headers":{"X-Codex-Primary-Reset-At":" 2000 "}}`, 2000},
		{"header-escaped-tab", `{"headers":{"X-Codex-Primary-Reset-At":"\t2000\t"}}`, 0},
		{"header-unicode-space", `{"headers":{"X-Codex-Primary-Reset-At":"\u20032000"}}`, 0},
		{"nested-escaped-tab", `{"error":{"reset_at":"\t2000\t"}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := quotaResetAtFromMessage([]byte(tc.payload), now); got != tc.want {
				t.Fatalf("quota reset numeric string %q parsed=%d want=%d", tc.name, got, tc.want)
			}
		})
	}
}

func TestProdex04383EscapedQuotaResetStringCannotExtendQuarantine(t *testing.T) {
	now := time.Unix(1_000, 0)
	for _, tc := range []struct {
		name, body string
		reset      int64
		wait       time.Duration
	}{
		{"valid-space", `{"error":{"code":"insufficient_quota"},"resets_at":" 2000 "}`, 2000, 1000 * time.Second},
		{"invalid-tab", `{"error":{"code":"insufficient_quota"},"resets_at":"\t2000\t"}`, 0, 5 * time.Minute},
		{"invalid-newline", `{"error":{"code":"insufficient_quota"},"resets_at":"\n2000"}`, 0, 5 * time.Minute},
		{"invalid-unicode", `{"error":{"code":"insufficient_quota"},"resets_at":"\u20032000"}`, 0, 5 * time.Minute},
		{"invalid-header-tab", `{"error":{"code":"insufficient_quota"},"headers":{"X-Codex-Primary-Reset-At":"\t2000\t"}}`, 0, 5 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := &Router{now: func() time.Time { return now }, maxInspect: 1 << 20,
				quarantine: make(map[string]quarantineState), quotaBlocked: make(map[string]bool), quotaChecks: make(map[quotaCheckKey]quotaCheck)}
			response := &proxymodel.Response{StatusCode: http.StatusForbidden, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(tc.body))}
			outcome, pending, err := router.classify(response, "openai")
			if err != nil {
				t.Fatal(err)
			}
			if pending != nil {
				defer pending.close()
			}
			if outcome.kind != responseRetry || !outcome.quota || outcome.quotaResetAt != tc.reset {
				t.Fatalf("escaped quota number changed reset: %+v want=%d", outcome, tc.reset)
			}
			router.applyRetryOutcome(t.Context(), "account-a", quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, outcome)
			if got := router.quarantineRemaining("account-a", now); got != tc.wait {
				t.Fatalf("escaped whitespace changed quarantine to %s want %s", got, tc.wait)
			}
		})
	}
}
