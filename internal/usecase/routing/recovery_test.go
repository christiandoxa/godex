package routing

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestFreshRequestWaitsForQuarantinedUsableAccount(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	gateway := &sequenceRoutingGateway{}
	quota := &cachedPressureQuota{byID: map[string]quotamodel.Availability{
		"a": {Ready: true},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/synthetic/a", Enabled: true, Provider: proxymodel.Provider{Kind: "openai"}}}, nil
		},
		PreferredAccount: "a",
		Now:              func() time.Time { return now },
		QuotaPreflight:   quota,
		Wait: func(ctx context.Context, delay time.Duration) error {
			if len(gateway.calls) != 0 {
				t.Fatalf("request dispatched before recovery wait: %v", gateway.calls)
			}
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("a", 15*time.Second)

	exchange, err := router.Forward(context.Background(), proxymodel.Request{
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-5.6"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "a" || exchange.Result.Response.StatusCode != http.StatusOK ||
		!reflect.DeepEqual(gateway.calls, []string{"a"}) || !reflect.DeepEqual(waits, []time.Duration{15 * time.Second}) {
		t.Fatalf("recovery owner/status/attempts/waits = %q/%d/%v/%v", exchange.Result.AccountID, exchange.Result.Response.StatusCode, gateway.calls, waits)
	}
}

func TestFreshRecoveryReloadsCandidatesAfterWait(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true, Provider: proxymodel.Provider{Kind: "openai"}}}
	accountLoads := 0
	var waits []time.Duration
	quota := &cachedPressureQuota{byID: map[string]quotamodel.Availability{
		"account-a": {Ready: true},
		"account-b": {Ready: true},
	}}
	gateway := &sequenceRoutingGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			accountLoads++
			return accounts, nil
		},
		QuotaPreflight: quota,
		Now:            func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			accounts = []proxymodel.Account{{ID: "account-b", Home: "/b", Enabled: true, Provider: proxymodel.Provider{Kind: "openai"}}}
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("account-a", time.Second)

	exchange, err := router.Forward(context.Background(), proxymodel.Request{
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-5.6"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.Response.StatusCode != http.StatusOK || exchange.Result.AccountID != "account-b" ||
		accountLoads < 2 || !reflect.DeepEqual(waits, []time.Duration{time.Second}) ||
		!reflect.DeepEqual(gateway.calls, []string{"account-b"}) {
		t.Fatalf("post-wait selection/status/loads/waits/dispatches = %q/%d/%d/%v/%v",
			exchange.Result.AccountID, exchange.Result.Response.StatusCode, accountLoads, waits, gateway.calls)
	}
}

func TestFreshTransientRecoveryContinuesPastSixteenSweepsWhileCandidateRemainsEligible(t *testing.T) {
	start := time.Unix(100, 0)
	now := start
	var waits []time.Duration
	fixtures := make([]routingResponseFixture, 18)
	for index := range fixtures[:17] {
		fixtures[index] = routingResponseFixture{status: http.StatusServiceUnavailable}
	}
	fixtures[17] = routingResponseFixture{status: http.StatusOK, body: `{"id":"recovered"}`}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{"a": fixtures}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/synthetic/a", Enabled: true}}, nil
		},
		Now: func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{RequestID: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	var waited time.Duration
	for _, delay := range waits {
		if delay <= 0 || delay > maxFreshRecoveryWait {
			t.Fatalf("recovery wait = %s, want a positive wait capped at %s", delay, maxFreshRecoveryWait)
		}
		waited += delay
	}
	if exchange.Result.Response.StatusCode != http.StatusOK || exchange.Result.Failed ||
		len(gateway.calls) != 18 || len(waits) < 17 ||
		waited <= 30*time.Second || !now.Equal(start.Add(waited)) {
		t.Fatalf("recovery status/failure/attempts/waits/time = %d/%t/%d/%d/%s", exchange.Result.Response.StatusCode, exchange.Result.Failed, len(gateway.calls), len(waits), waited)
	}
	for index, account := range gateway.calls {
		if account != "a" {
			t.Fatalf("attempt %d selected %q, want eligible account a", index+1, account)
		}
	}
}

func TestFreshRecoveryDoesNotImposeFormerFixedAttemptBudget(t *testing.T) {
	const expectedBudget = 64
	const candidateCount = expectedBudget + 1
	now := time.Unix(100, 0)
	var waits []time.Duration
	accounts := make([]proxymodel.Account, candidateCount)
	responses := make(map[string][]routingResponseFixture, candidateCount)
	for index := range accounts {
		id := fmt.Sprintf("account-%03d", index)
		accounts[index] = proxymodel.Account{ID: id, Home: "/synthetic/" + id, Enabled: true, RouteOrder: index + 1}
		status := http.StatusServiceUnavailable
		if index == candidateCount-1 {
			status = http.StatusOK
		}
		responses[id] = []routingResponseFixture{{status: status}}
	}
	gateway := &sequenceRoutingGateway{responses: responses}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return accounts, nil
		},
		Now: func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.Response.StatusCode != http.StatusOK || exchange.Result.Failed ||
		len(gateway.calls) != candidateCount || len(waits) != 0 || gateway.calls[len(gateway.calls)-1] != "account-064" {
		t.Fatalf("budget status/failure/attempts/waits = %d/%t/%d/%d", exchange.Result.Response.StatusCode, exchange.Result.Failed, len(gateway.calls), len(waits))
	}
}

func TestFreshRouteDoesNotWaitOnPreviouslyQuarantinedAuthFailure(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
		"a": {{status: http.StatusUnauthorized}, {status: http.StatusUnauthorized}},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/synthetic/a", Enabled: true}}, nil
		},
		Now: func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	if _, err := router.Forward(context.Background(), proxymodel.Request{}); err == nil {
		t.Fatal("second request unexpectedly recovered an auth-failed account")
	}
	if len(gateway.calls) != 2 || len(waits) != 0 {
		t.Fatalf("auth retry dispatches/waits = %d/%v, want 2/none", len(gateway.calls), waits)
	}
}
