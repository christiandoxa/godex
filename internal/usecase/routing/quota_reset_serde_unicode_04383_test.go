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

// Tagged 0.438.3 quota_reset_at_from_json_message uses a *complete*
// serde_json::Value before the Mojo epoch planner examines any reset
// field. Go RawMessage would otherwise accept malformed unrelated JSON.
func TestProdex04383QuotaResetRequiresValidCompleteSerdeDocument(t *testing.T) {
	now := time.Unix(1_000, 0)
	prefix := `{"resets_at":2000,"noise":`
	for _, tc := range []struct {
		name, message string
		want          int64
	}{
		{"canonical", `{"resets_at":2000}`, 2000},
		{"finite-large", prefix + `1e308}`, 2000},
		{"valid-surrogate", prefix + `"\ud83d\ude00"}`, 2000},
		{"invalid-utf8", prefix + `"bad-` + string([]byte{0xff}) + `"}`, 0},
		{"invalid-high-surrogate", prefix + `"\ud800"}`, 0},
		{"invalid-low-surrogate", prefix + `"\udc00"}`, 0},
		{"overflow-number", prefix + `1e309}`, 0},
		{"overdepth", prefix + strings.Repeat(`{"item":`, 128) + `0` + strings.Repeat(`}`, 128) + `}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := quotaResetAtFromMessage([]byte(tc.message), now); got != tc.want {
				t.Fatalf("quota reset %s=%d want=%d", tc.name, got, tc.want)
			}
		})
	}
}

// The tagged quota JSON epoch planner uses byte-exact ASCII key matching
// (with uppercase ASCII support) on serde-canonicalized names. Go's
// strings.EqualFold would treat long-s U+017F as ASCII 's'.
func TestProdex04383QuotaResetHeaderNamesRejectUnicodeLongS(t *testing.T) {
	now := time.Unix(1_000, 0)
	for _, tc := range []struct {
		name, payload string
		want          int64
	}{
		{"canonical-primary", `{"headers":{"X-Codex-Primary-Used-Percent":100,"X-Codex-Primary-Reset-At":1700}}`, 1700},
		{"upper-ASCII-primary", `{"headers":{"X-CODEX-PRIMARY-USED-PERCENT":100,"X-CODEX-PRIMARY-RESET-AT":1700}}`, 1700},
		{"confusable-used-percent", `{"headers":{"X-Codex-Primary-Uſed-Percent":100,"X-Codex-Primary-Reset-At":1700,"X-Codex-Secondary-Used-Percent":100,"X-Codex-Secondary-Reset-At":1800}}`, 1800},
		{"confusable-reset-name", `{"headers":{"X-Codex-Primary-Used-Percent":100,"X-Codex-Primary-Reſet-At":1700}}`, 0},
		{"valid-secondary", `{"headers":{"X-Codex-Secondary-Used-Percent":100,"X-Codex-Secondary-Reset-At":1800}}`, 1800},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := quotaResetAtFromMessage([]byte(tc.payload), now); got != tc.want {
				t.Fatalf("unicode confusable quota-reset header %s=%d want=%d", tc.name, got, tc.want)
			}
		})
	}
}

// A quota-classified HTTP 403 must keep its short fallback quarantine when
// the reset time came from malformed JSON. Conversely, a valid exhausted
// secondary window must supersede a spoofed primary used-percent key.
func TestProdex04383MalformedQuotaResetDoesNotExtendProfileQuarantine(t *testing.T) {
	now := time.Unix(1_000, 0)
	for _, tc := range []struct {
		name, body   string
		wantReset    int64
		wantDuration time.Duration
	}{
		{"valid-top-level", `{"error":{"code":"insufficient_quota"},"resets_at":2000}`, 2000, 1000 * time.Second},
		{"unpaired-reset", `{"error":{"code":"insufficient_quota"},"resets_at":2000,"unused":"\ud800"}`, 0, 5 * time.Minute},
		{"nonfinite-reset", `{"error":{"code":"insufficient_quota"},"resets_at":2000,"unused":1e309}`, 0, 5 * time.Minute},
		{"unicode-header-spoof", `{"error":{"code":"insufficient_quota"},"headers":{"X-Codex-Primary-Uſed-Percent":100,"X-Codex-Primary-Reset-At":1700,"X-Codex-Secondary-Used-Percent":100,"X-Codex-Secondary-Reset-At":1800}}`, 1800, 800 * time.Second},
		{"ASCII-header-control", `{"error":{"code":"insufficient_quota"},"headers":{"X-Codex-Primary-Used-Percent":100,"X-Codex-Primary-Reset-At":1700,"X-Codex-Secondary-Used-Percent":100,"X-Codex-Secondary-Reset-At":1800}}`, 1700, 700 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := &Router{now: func() time.Time { return now }, maxInspect: 1 << 20,
				quarantine: make(map[string]quarantineState), quotaBlocked: make(map[string]bool), quotaChecks: make(map[quotaCheckKey]quotaCheck)}
			response := &proxymodel.Response{StatusCode: http.StatusForbidden,
				Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.body))}
			outcome, pending, err := router.classify(response, "openai")
			if err != nil {
				t.Fatal(err)
			}
			if pending != nil {
				defer pending.close()
			}
			if outcome.kind != responseRetry || !outcome.quota || outcome.quotaResetAt != tc.wantReset {
				t.Fatalf("HTTP quota result used untrusted reset evidence: %+v wantReset=%d", outcome, tc.wantReset)
			}
			router.applyRetryOutcome(t.Context(), "account-a", quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, outcome)
			if remaining := router.quarantineRemaining("account-a", now); remaining != tc.wantDuration {
				t.Fatalf("invalid reset changed quarantine duration to %s want %s", remaining, tc.wantDuration)
			}
		})
	}
}
