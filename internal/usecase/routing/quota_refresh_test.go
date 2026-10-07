package routing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type routingQuotaAvailabilityFake struct {
	byID        map[string]quotamodel.Availability
	bySelection map[quotamodel.Selection]quotamodel.Availability
	errID       map[string]error
	calls       []string
	selections  []quotamodel.Selection
}

func (fake *routingQuotaAvailabilityFake) AvailabilityForRoute(_ context.Context, account accountentity.Account, selection quotamodel.Selection) (quotamodel.Availability, error) {
	fake.calls = append(fake.calls, account.ID)
	fake.selections = append(fake.selections, selection)
	if availability, ok := fake.bySelection[selection]; ok {
		return availability, fake.errID[account.ID]
	}
	return fake.byID[account.ID], fake.errID[account.ID]
}

func TestFreshRoutingRefreshesLaunchQuotaExclusionAfterQuotaFailure(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true, EligibleAfter: now.Add(time.Hour)},
	}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
		"account-a": {
			{status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`},
			{status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`},
		},
		"account-b": {{status: http.StatusOK, body: `{"id":"ready"}`}},
	}}
	quota := &routingQuotaAvailabilityFake{byID: map[string]quotamodel.Availability{
		"account-b": {Ready: true},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a", Now: func() time.Time { return now },
		QuotaPreflight: quota,
		Accounts:       func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-5.6-luna"}
	for range 2 {
		exchange, err := router.Forward(context.Background(), proxymodel.Request{QuotaSelection: selection})
		if err != nil {
			t.Fatal(err)
		}
		if exchange.Result.AccountID != "account-b" || exchange.Result.Response.StatusCode != http.StatusOK {
			t.Fatalf("fallback result = %#v", exchange.Result)
		}
		if err := exchange.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(gateway.calls, ",") != "account-a,account-b,account-b" || strings.Join(quota.calls, ",") != "account-b" {
		t.Fatalf("gateway/quota calls = %v / %v", gateway.calls, quota.calls)
	}
	if len(quota.selections) != 1 || quota.selections[0] != selection {
		t.Fatalf("quota selections = %#v", quota.selections)
	}
}

func TestFreshRoutingKeepsQuotaExcludedProfileWhenRefreshStillBlocksIt(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true, EligibleAfter: now.Add(time.Hour)},
	}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
		"account-a": {{status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`}},
	}}
	quota := &routingQuotaAvailabilityFake{byID: map[string]quotamodel.Availability{
		"account-b": {RetryAt: now.Add(2 * time.Hour)},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a", Now: func() time.Time { return now },
		QuotaPreflight: quota,
		Accounts:       func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-a" || exchange.Result.Response.StatusCode != http.StatusForbidden {
		t.Fatalf("quota response = %#v", exchange.Result)
	}
	if strings.Join(gateway.calls, ",") != "account-a" || strings.Join(quota.calls, ",") != "account-b" {
		t.Fatalf("gateway/quota calls = %v / %v", gateway.calls, quota.calls)
	}
}

func TestFreshRoutingFailsOpenWhenQuotaRefreshFails(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true, EligibleAfter: now.Add(time.Hour)},
	}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
		"account-a": {{status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`}},
		"account-b": {{status: http.StatusOK, body: `{"id":"ready"}`}},
	}}
	quota := &routingQuotaAvailabilityFake{errID: map[string]error{
		"account-b": errors.New("synthetic quota probe failure"),
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a", Now: func() time.Time { return now },
		QuotaPreflight: quota,
		Accounts:       func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-b" || exchange.Result.Response.StatusCode != http.StatusOK {
		t.Fatalf("fail-open fallback = %#v", exchange.Result)
	}
	if strings.Join(gateway.calls, ",") != "account-a,account-b" || strings.Join(quota.calls, ",") != "account-b" {
		t.Fatalf("gateway/quota calls = %v / %v", gateway.calls, quota.calls)
	}
}

func TestBoundRoutingRefreshesStaleQuotaExclusion(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}
	gateway := &sequenceRoutingGateway{}
	quota := &routingQuotaAvailabilityFake{byID: map[string]quotamodel.Availability{
		"account-a": {Ready: true},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, Now: func() time.Time { return now }, QuotaPreflight: quota,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := proxymodel.Request{Header: http.Header{"Thread-Id": {"thread-a"}}}
	first, err := router.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	accounts[0].EligibleAfter = now.Add(time.Hour)
	second, err := router.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Result.AccountID != "account-a" || strings.Join(gateway.calls, ",") != "account-a,account-a" || strings.Join(quota.calls, ",") != "account-a" {
		t.Fatalf("bound owner/quota/gateway = %#v / %v / %v", second.Result, quota.calls, gateway.calls)
	}
}

func TestQuotaRefreshCacheSeparatesRouteSelections(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true, EligibleAfter: now.Add(time.Hour)}}
	response := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-5.6-sol"}
	standard := quotamodel.Selection{RouteKind: quotamodel.RouteKindStandard, RequestedModel: "gpt-5.6-sol"}
	quota := &routingQuotaAvailabilityFake{bySelection: map[quotamodel.Selection]quotamodel.Availability{
		response: {Ready: true},
		standard: {RetryAt: now.Add(time.Hour)},
	}}
	gateway := &sequenceRoutingGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway, Now: func() time.Time { return now }, QuotaPreflight: quota,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{QuotaSelection: response})
	if err != nil {
		t.Fatal(err)
	}
	if err := exchange.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := router.Forward(context.Background(), proxymodel.Request{QuotaSelection: standard}); err == nil {
		t.Fatal("route-specific quota cache reused response availability")
	}
	if strings.Join(gateway.calls, ",") != "account-a" || strings.Join(quota.calls, ",") != "account-a,account-a" {
		t.Fatalf("gateway/quota calls = %v / %v", gateway.calls, quota.calls)
	}
}

func TestRequestQuotaGateUsesFreshCachedFailureOnlyWhenPoolHasAlternative(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true},
	}
	gateway := &sequenceRoutingGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a", Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-5.6-sol"}
	router.cacheQuotaFailure("account-a", selection, time.Minute)

	exchange, err := router.Forward(context.Background(), proxymodel.Request{QuotaSelection: selection})
	if err != nil {
		t.Fatal(err)
	}
	if err := exchange.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(gateway.calls, ",") != "account-b" {
		t.Fatalf("upstream candidates = %v, want only cached-ready account-b", gateway.calls)
	}

	blockedGateway := &sequenceRoutingGateway{}
	blockedRouter, err := NewRouter(Config{
		Gateway: blockedGateway, PreferredAccount: "account-a", Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	blockedRouter.cacheQuotaFailure("account-a", selection, time.Minute)
	blockedRouter.cacheQuotaFailure("account-b", selection, time.Minute)
	allBlocked, err := blockedRouter.Forward(context.Background(), proxymodel.Request{QuotaSelection: selection})
	if err != nil {
		t.Fatal(err)
	}
	if err := allBlocked.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(blockedGateway.calls, ",") != "account-a" {
		t.Fatalf("all-blocked pool did not fail open: %v", blockedGateway.calls)
	}

	otherSelection := quotamodel.Selection{
		RouteKind: quotamodel.RouteKindStandard, RequestedModel: selection.RequestedModel,
	}
	if got := blockedRouter.requestCandidates(accounts, otherSelection, now); len(got) != 2 {
		t.Fatalf("Responses quota cache filtered another route's candidates: %#v", got)
	}
}

type delayedQuotaAvailabilityFake struct {
	delay     time.Duration
	completed bool
}

func (fake *delayedQuotaAvailabilityFake) AvailabilityForRoute(
	ctx context.Context,
	_ accountentity.Account,
	_ quotamodel.Selection,
) (quotamodel.Availability, error) {
	timer := time.NewTimer(fake.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		fake.completed = true
		return quotamodel.Availability{Ready: true}, nil
	case <-ctx.Done():
		return quotamodel.Availability{}, ctx.Err()
	}
}

func TestProdex04358ColdStartQuotaProbeWaitsForRealProgress(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{{
		ID: "account-delayed", Home: "/delayed", Enabled: true,
		EligibleAfter: now.Add(time.Hour),
	}}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
		"account-delayed": {{status: http.StatusOK, body: `{"id":"ready-after-probe"}`}},
	}}
	quota := &delayedQuotaAvailabilityFake{delay: 30 * time.Millisecond}
	router, err := NewRouter(Config{
		Gateway: gateway, Now: func() time.Time { return now }, QuotaPreflight: quota,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	exchange, err := router.Forward(ctx, proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if !quota.completed {
		t.Fatal("route continued before the cold-start quota probe made real progress")
	}
	if exchange.Result.AccountID != "account-delayed" || exchange.Result.Response.StatusCode != http.StatusOK {
		t.Fatalf("route result = %#v", exchange.Result)
	}
}
