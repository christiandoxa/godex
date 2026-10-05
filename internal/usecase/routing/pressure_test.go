package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

type pressureGateway struct{ owners []string }

func (gateway *pressureGateway) Execute(_ context.Context, _ proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.owners = append(gateway.owners, account.ID)
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\"}\n\n")),
	}, nil
}

type routeHealthGateway struct {
	owners  []string
	failing string
}

func (gateway *routeHealthGateway) Execute(_ context.Context, _ proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.owners = append(gateway.owners, account.ID)
	status := http.StatusOK
	body := `data: {"type":"response.output_text.delta"}` + "\n\n"
	contentType := "text/event-stream"
	if account.ID == gateway.failing {
		status = http.StatusServiceUnavailable
		body = `{}`
		contentType = "application/json"
	}
	return &proxymodel.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

type cachedPressureQuota struct {
	byID  map[string]quotamodel.Availability
	calls []string
}

func (quota *cachedPressureQuota) AvailabilityForRoute(_ context.Context, account accountentity.Account, _ quotamodel.Selection) (quotamodel.Availability, error) {
	quota.calls = append(quota.calls, account.ID)
	return quota.byID[account.ID], nil
}

func (quota *cachedPressureQuota) CachedAvailabilityForRoute(account accountentity.Account, _ quotamodel.Selection, _ time.Time) (quotamodel.Availability, bool) {
	availability, ok := quota.byID[account.ID]
	return availability, ok
}

func TestFreshCandidatesPreferLeastLoadedProfileUntilResponseCloses(t *testing.T) {
	gateway := &pressureGateway{}
	router, err := NewRouter(Config{
		Gateway:          gateway,
		PreferredAccount: "account-a",
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "account-a", Home: "/a", Enabled: true},
				{ID: "account-b", Home: "/b", Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := router.Forward(context.Background(), proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := router.Forward(context.Background(), proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	if got := strings.Join(gateway.owners, ","); got != "account-a,account-b" {
		t.Fatalf("owners before response close = %q", got)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := router.Forward(context.Background(), proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		_ = second.Close()
		t.Fatal(err)
	}
	defer second.Close()
	defer third.Close()
	if got := strings.Join(gateway.owners, ","); got != "account-a,account-b,account-a" {
		t.Fatalf("owners after first response close = %q", got)
	}
}

func TestCachedRouteQuotaSkipsExhaustedProfileWithReadyAlternative(t *testing.T) {
	now := time.Unix(100, 0)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-5.6-sol"}
	quota := &cachedPressureQuota{byID: map[string]quotamodel.Availability{
		"account-a": {RetryAt: now.Add(time.Minute), Pressure: pressureScore(3, 1_000_000)},
		"account-b": {Ready: true, Pressure: pressureScore(0, 10)},
	}}
	gateway := &pressureGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway, QuotaPreflight: quota, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}, {ID: "account-b", Home: "/b", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{QuotaSelection: selection})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.owners, ","); got != "account-b" || len(quota.calls) != 0 {
		t.Fatalf("upstream owners = %q; live quota calls = %v", got, quota.calls)
	}
}

func TestQuotaPressurePrecedesInflightAndHealthAdjustsTies(t *testing.T) {
	now := time.Unix(100, 0)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	quota := &cachedPressureQuota{byID: map[string]quotamodel.Availability{
		"account-a": {Ready: true, Pressure: pressureScore(1, 200)},
		"account-b": {Ready: true, Pressure: pressureScore(0, 100)},
	}}
	accounts := []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}, {ID: "account-b", Home: "/b", Enabled: true}}
	router, err := NewRouter(Config{
		QuotaPreflight: quota, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	release := router.beginRequestInFlight("account-b", quotamodel.Selection{RouteKind: quotamodel.RouteKindStandard})
	if got := router.requestCandidates(accounts, selection, now)[0].ID; got != "account-b" {
		release()
		t.Fatalf("candidate with more quota pressure won: %q", got)
	}
	release()

	quota.byID["account-a"] = quotamodel.Availability{Ready: true, Pressure: pressureScore(0, 100)}
	quota.byID["account-b"] = quotamodel.Availability{Ready: true, Pressure: pressureScore(0, 100)}
	healthSelection := quotamodel.Selection{RouteKind: quotamodel.RouteKindStandard}
	router.recordRouteFailure(context.Background(), "account-a", healthSelection)
	if got := router.requestCandidates(accounts, healthSelection, now)[0].ID; got != "account-b" {
		t.Fatalf("less healthy account won tie: %q", got)
	}
}

func TestInflightSoftLimitFallsBackWhenEveryProfileIsBusy(t *testing.T) {
	if defaultInflightSoftLimit != 4 {
		t.Fatalf("default soft limit = %d, want the tagged Prodex limit of 4", defaultInflightSoftLimit)
	}
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}, {ID: "account-b", Home: "/b", Enabled: true}}
	router, err := NewRouter(Config{
		Now:      func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	var release []func()
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindStandard}
	for range defaultInflightSoftLimit {
		release = append(release, router.beginRequestInFlight("account-a", selection))
	}
	if got := router.requestCandidates(accounts, selection, now); len(got) != 2 || got[0].ID != "account-b" {
		t.Fatalf("soft-limited candidates = %#v", got)
	}
	for range defaultInflightSoftLimit {
		release = append(release, router.beginRequestInFlight("account-b", selection))
	}
	if got := router.requestCandidates(accounts, selection, now); len(got) != 2 {
		t.Fatalf("all-soft-limited fallback candidates = %#v", got)
	}
	for _, done := range release {
		done()
	}
}

func TestExecuteTracksTaggedWeightedInflightUntilBodyClose(t *testing.T) {
	router := &Router{gateway: &pressureGateway{}}
	for _, test := range []struct {
		route  quotamodel.RouteKind
		weight int
	}{
		{route: quotamodel.RouteKindResponses, weight: 2},
		{route: quotamodel.RouteKindWebSocket, weight: 2},
		{route: quotamodel.RouteKindCompact, weight: 1},
		{route: quotamodel.RouteKindStandard, weight: 1},
	} {
		response, err := router.execute(context.Background(), proxymodel.Request{
			QuotaSelection: quotamodel.Selection{RouteKind: test.route},
		}, proxymodel.Account{ID: "account-a"})
		if err != nil {
			t.Fatalf("execute route %v: %v", test.route, err)
		}
		if got := router.inflight["account-a"]; got != test.weight {
			t.Errorf("route %v in-flight weight = %d, want %d", test.route, got, test.weight)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatalf("close route %v response: %v", test.route, err)
		}
		if got := router.inflight["account-a"]; got != 0 {
			t.Errorf("route %v in-flight after release = %d, want 0", test.route, got)
		}
	}
}

func TestTransientBackoffDefersCandidatesAndKeepsThemAsFallbacks(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true},
		{ID: "account-c", Home: "/c", Enabled: true},
	}
	router, err := NewRouter(Config{
		Now:              func() time.Time { return now },
		PreferredAccount: "account-a",
		Accounts:         func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("account-a", 2*time.Minute)
	router.quarantineAccount("account-b", time.Minute)

	ordered := router.requestCandidates(accounts, quotamodel.Selection{}, now)
	if got := strings.Join([]string{ordered[0].ID, ordered[1].ID, ordered[2].ID}, ","); got != "account-c,account-b,account-a" {
		t.Fatalf("candidates by transient backoff = %q", got)
	}
}

func TestRouteHealthLoadsFromPersistenceAndStaysRouteScoped(t *testing.T) {
	now := time.Unix(100, 0)
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	if _, err := store.AdjustRouteHealth(context.Background(), "account-a", "responses", 1, now); err != nil {
		t.Fatal(err)
	}
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true},
	}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, RoutingState: routingrepo.NewStore(root),
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := router.requestCandidates(accounts, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, now)[0].ID; got != "account-b" {
		t.Fatalf("persisted route-health candidate = %q, want account-b", got)
	}
	if got := router.requestCandidates(accounts, quotamodel.Selection{RouteKind: quotamodel.RouteKindStandard}, now)[0].ID; got != "account-a" {
		t.Fatalf("health score leaked across routes: candidate = %q", got)
	}
}

func TestFreshRouteFailurePersistsPenaltyAcrossRouterRestart(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true},
	}
	newRouter := func(gateway *routeHealthGateway) *Router {
		router, err := NewRouter(Config{
			Gateway: gateway, Now: func() time.Time { return now }, RoutingState: routingrepo.NewStore(root),
			Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		return router
	}

	firstGateway := &routeHealthGateway{failing: "account-a"}
	first, err := newRouter(firstGateway).Forward(context.Background(), proxymodel.Request{
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(firstGateway.owners, ","); got != "account-a,account-b" {
		t.Fatalf("first routing attempts = %q", got)
	}
	scores, err := routingrepo.NewStore(root).LoadRouteHealth(context.Background(), now)
	if err != nil || len(scores) != 1 || scores[0].AccountID != "account-a" {
		t.Fatalf("persisted health after normal success = %+v, error = %v", scores, err)
	}

	secondGateway := &routeHealthGateway{}
	second, err := newRouter(secondGateway).Forward(context.Background(), proxymodel.Request{
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if got := strings.Join(secondGateway.owners, ","); got != "account-b" {
		t.Fatalf("post-restart route choice = %q, want account-b", got)
	}
}

func TestBoundTransientFailureUpdatesPersistedRouteHealth(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0)
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	account := proxymodel.Account{ID: "account-a", Home: "/a", Enabled: true}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, RoutingState: store,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	response := &proxymodel.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{}`)),
	}
	forwarded, err := router.handleBoundResponse(ctx, proxymodel.Request{
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}, []proxymodel.Account{account}, account, response)
	if err != nil {
		t.Fatal(err)
	}
	if err := forwarded.Response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	scores, err := routingrepo.NewStore(root).LoadRouteHealth(ctx, now)
	if err != nil || len(scores) != 1 || scores[0].AccountID != account.ID || scores[0].Score != 2 {
		t.Fatalf("bound-owner route health = %+v, error = %v", scores, err)
	}
}

func pressureScore(band uint8, total int64) quotamodel.Pressure {
	return quotamodel.Pressure{
		Known: true, Band: band, Total: total, Weekly: total, FiveHour: total,
		ReserveFloor: 50, WeeklyRemaining: 50, FiveHourRemaining: 50,
		WeeklyResetAt: 1000, FiveHourResetAt: 1000,
	}
}

func TestProdex04355OpenAIProviderPriorityPrecedesAdapterOnEqualLoad(t *testing.T) {
	now := time.Unix(200, 0)
	accounts := []proxymodel.Account{
		{ID: "adapter", Home: "/adapter", Enabled: true, RouteOrder: 1, Provider: proxymodel.Provider{Kind: "anthropic"}},
		{ID: "openai", Home: "/openai", Enabled: true, RouteOrder: 2},
	}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ordered := router.requestCandidates(accounts, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, now)
	if len(ordered) != 2 || ordered[0].ID != "openai" {
		t.Fatalf("equal-load provider order = %#v, want native OpenAI before adapter", ordered)
	}
}

func TestProdex04355OverloadAddsTwoRouteHealthPoints(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(300, 0)
	store := routingrepo.NewStore(t.TempDir())
	account := proxymodel.Account{ID: "account-a", Home: "/a", Enabled: true}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, RoutingState: store,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	response := &proxymodel.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)),
	}
	forwarded, err := router.handleBoundResponse(ctx, proxymodel.Request{
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}, []proxymodel.Account{account}, account, response)
	if err != nil {
		t.Fatal(err)
	}
	if err := forwarded.Response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	scores, err := store.LoadRouteHealth(ctx, now)
	if err != nil || len(scores) != 1 || scores[0].Score != 2 {
		t.Fatalf("overload route health = %+v, error = %v; want score 2", scores, err)
	}
}

func TestProdex04355RateLimitDoesNotAddRouteHealthPenalty(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(400, 0)
	store := routingrepo.NewStore(t.TempDir())
	account := proxymodel.Account{ID: "account-a", Home: "/a", Enabled: true}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, RoutingState: store,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	response := &proxymodel.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"code":"rate_limit_exceeded"}}`)),
	}
	forwarded, err := router.handleBoundResponse(ctx, proxymodel.Request{
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}, []proxymodel.Account{account}, account, response)
	if err != nil {
		t.Fatal(err)
	}
	if err := forwarded.Response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	scores, err := store.LoadRouteHealth(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 0 {
		t.Fatalf("rate limit added route-health penalty: %+v", scores)
	}
}
