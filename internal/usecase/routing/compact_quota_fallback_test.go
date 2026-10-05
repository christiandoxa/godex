package routing

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestCompactQuotaFallbackDoesNotRetryRequestExcludedGlobalCandidate(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "profile-a", Home: "/synthetic/a", Enabled: true},
		{ID: "profile-b", Home: "/synthetic/b", Enabled: true},
	}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
		"profile-a": {{status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`}},
		"profile-b": {
			{status: http.StatusServiceUnavailable, body: `{"error":{"code":"temporarily_unavailable"}}`},
			{status: http.StatusOK, body: `{"id":"recovered"}`},
		},
	}}
	quota := &cachedPressureQuota{byID: map[string]quotamodel.Availability{
		"profile-a": {Ready: true},
		"profile-b": {Ready: true},
	}}
	var waits []time.Duration
	router, err := NewRouter(Config{
		Gateway:        gateway,
		QuotaPreflight: quota,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return accounts, nil
		},
		PreferredAccount: "profile-b",
		Now:              func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindCompact}
	exchange, err := router.Forward(context.Background(), proxymodel.Request{
		Path:           "/backend-api/codex/responses/compact",
		Header:         make(http.Header),
		QuotaSelection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "profile-a" || !exchange.Result.Failed || exchange.Result.Response.StatusCode != http.StatusForbidden {
		t.Fatalf("Compact quota result = %#v; want profile A's terminal quota response", exchange.Result)
	}
	if strings.Join(gateway.calls, ",") != "profile-b,profile-a" || len(waits) != 0 {
		t.Fatalf("Compact attempts/waits = %v/%v; want one attempt per profile without retrying excluded B", gateway.calls, waits)
	}
	if !hasAccount(router.requestCandidates(accounts, selection, now), "profile-b") {
		t.Fatal("profile B should remain quota eligible globally after request-local exclusion")
	}
}

func hasAccount(accounts []proxymodel.Account, id string) bool {
	for _, account := range accounts {
		if account.ID == id {
			return true
		}
	}
	return false
}
