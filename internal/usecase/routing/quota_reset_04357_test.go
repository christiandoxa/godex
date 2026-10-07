package routing

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04357QuotaJSONResetControlsQuarantineDuration(t *testing.T) {
	now := time.Unix(1_000, 0)
	router := &Router{
		now:          func() time.Time { return now },
		maxInspect:   1 << 20,
		quarantine:   make(map[string]quarantineState),
		quotaBlocked: make(map[string]bool),
		quotaChecks:  make(map[quotaCheckKey]quotaCheck),
	}
	cases := []struct {
		name string
		body string
		want time.Duration
	}{
		{
			name: "nested reset",
			body: `{"error":{"code":"insufficient_quota","resets_at":1600}}`,
			want: 10 * time.Minute,
		},
		{
			name: "top-level precedence",
			body: `{"resets_at":1700,"reset_at":1800,"error":{"code":"insufficient_quota","resets_at":1900}}`,
			want: 700 * time.Second,
		},
		{
			name: "primary header exhausted",
			body: `{"error":{"code":"insufficient_quota"},"headers":{"X-Codex-Primary-Used-Percent":"100","X-Codex-Primary-Reset-At":"2000","X-Codex-Secondary-Used-Percent":"100","X-Codex-Secondary-Reset-At":"2100"}}`,
			want: 1000 * time.Second,
		},
		{
			name: "secondary wins when primary is not exhausted",
			body: `{"error":{"code":"insufficient_quota"},"headers":{"X-Codex-Primary-Used-Percent":99,"X-Codex-Primary-Reset-At":2000,"X-Codex-Secondary-Used-Percent":100,"X-Codex-Secondary-Reset-At":2100}}`,
			want: 1100 * time.Second,
		},
		{
			name: "case-folded header keys",
			body: `{"error":{"code":"insufficient_quota"},"headers":{"x-cOdEx-pRiMaRy-uSeD-pErCeNt":"100","X-cOdEx-pRiMaRy-rEsEt-aT":"2200"}}`,
			want: 1200 * time.Second,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := &proxymodel.Response{
				StatusCode: http.StatusForbidden,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(testCase.body)),
			}
			outcome, pending, err := router.classify(response, "openai")
			if err != nil {
				t.Fatal(err)
			}
			if pending != nil {
				defer pending.close()
			}
			if !outcome.quota || outcome.kind != responseRetry {
				t.Fatalf("classification = %#v", outcome)
			}
			router.applyRetryOutcome(t.Context(), "account-a", quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, outcome)
			if got := router.quarantineRemaining("account-a", now); got != testCase.want {
				t.Fatalf("quarantine = %s, want %s", got, testCase.want)
			}
			router.clearQuarantine("account-a")
		})
	}
}

func TestProdex04357QuotaQuarantineFallsBackToKnownResetThenFiveMinutes(t *testing.T) {
	now := time.Unix(2_000, 0)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-6-luna"}
	router := &Router{
		now:          func() time.Time { return now },
		quarantine:   make(map[string]quarantineState),
		quotaBlocked: make(map[string]bool),
		quotaChecks:  make(map[quotaCheckKey]quotaCheck),
	}
	router.quotaChecks[quotaCheckKey{accountID: "account-a", selection: selection}] = quotaCheck{
		checkedAt: now.Add(-10 * time.Minute),
		retryAt:   now.Add(20 * time.Minute),
	}
	outcome := responseOutcome{kind: responseRetry, quarantine: 30 * time.Second, quota: true}
	router.applyRetryOutcome(t.Context(), "account-a", selection, outcome)
	if got := router.quarantineRemaining("account-a", now); got != 20*time.Minute {
		t.Fatalf("known-reset quarantine = %s, want 20m", got)
	}

	router.clearQuarantine("account-a")
	delete(router.quotaChecks, quotaCheckKey{accountID: "account-a", selection: selection})
	router.applyRetryOutcome(t.Context(), "account-a", selection, outcome)
	if got := router.quarantineRemaining("account-a", now); got != 5*time.Minute {
		t.Fatalf("fallback quarantine = %s, want 5m", got)
	}
}
