package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04355FallbackBackoffTuplePrefersCircuitOnlyBeforeTransportOnly(t *testing.T) {
	now := time.Unix(200_000, 0)
	router, err := NewRouter(Config{Now: func() time.Time { return now }, Accounts: healthParityAccounts})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	routeKeyA := routeHealthKey{accountID: "account-a", route: "responses"}
	routeKeyB := routeHealthKey{accountID: "account-b", route: "responses"}
	router.routeCircuits[routeKeyA] = routingentity.RouteCircuit{AccountID: "account-a", Route: "responses", UntilUnix: now.Add(30 * time.Second).Unix()}
	router.transportBackoffs[routeKeyB] = routingentity.TransportBackoff{AccountID: "account-b", Route: "responses", UntilUnix: now.Add(10 * time.Second).Unix()}
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true, RouteOrder: 1},
		{ID: "account-b", Home: "/b", Enabled: true, RouteOrder: 2},
	}
	ordered := router.orderCandidates(accounts, selection, now)
	if len(ordered) != 2 || ordered[0].ID != "account-a" {
		t.Fatalf("fallback backoff order = %#v, want circuit-only account-a first", []string{ordered[0].ID, ordered[1].ID})
	}
}

type promptCacheParityGateway struct {
	mu       sync.Mutex
	accounts []string
}

func (gateway *promptCacheParityGateway) Execute(_ context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	gateway.accounts = append(gateway.accounts, account.ID)
	gateway.mu.Unlock()
	body := `{"id":"resp-` + account.ID + `","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`
	return &proxymodel.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestProdex04355PromptCacheOwnerPrecedesStableRouteOrder(t *testing.T) {
	now := time.Unix(210_000, 0)
	gateway := &promptCacheParityGateway{}
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true, RouteOrder: 1},
		{ID: "account-b", Home: "/b", Enabled: true, RouteOrder: 2},
	}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, Gateway: gateway, PreferredAccount: "account-b",
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := proxymodel.Request{
		RequestID: 1, Method: http.MethodPost, Path: "/responses",
		Body:           []byte(`{"prompt_cache_key":"cache-1","input":"hello"}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}
	first, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	request.RequestID = 2
	second, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Close()
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.accounts) != 2 || gateway.accounts[0] != "account-b" || gateway.accounts[1] != "account-b" {
		t.Fatalf("prompt-cache routed accounts = %v, want [account-b account-b]", gateway.accounts)
	}
}

func TestProdex04355PreferredCurrentYieldsToPromptCacheOwner(t *testing.T) {
	now := time.Unix(220_000, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true, RouteOrder: 1},
		{ID: "account-b", Home: "/b", Enabled: true, RouteOrder: 2},
	}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, PreferredAccount: "account-b",
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.rememberPromptCacheOwner("account-a", "cache-1", now)
	request := proxymodel.Request{
		RequestID:      1,
		Body:           []byte(`{"prompt_cache_key":"cache-1","input":"hello"}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}
	ordered := router.requestCandidatesForRequest(accounts, request, now)
	if len(ordered) != 2 || ordered[0].ID != "account-a" {
		t.Fatalf("prompt-cache current order = %v, want owner account-a first", []string{ordered[0].ID, ordered[1].ID})
	}
}

func TestProdex04355PreferredCurrentYieldsToRouteHealthPenalty(t *testing.T) {
	now := time.Unix(230_000, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true, RouteOrder: 1},
		{ID: "account-b", Home: "/b", Enabled: true, RouteOrder: 2},
	}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, PreferredAccount: "account-b",
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	router.routeHealth[routeHealthKey{accountID: "account-b", route: "responses"}] = routingentity.RouteHealthScore{
		AccountID: "account-b", Route: "responses", Score: 1, UpdatedUnix: now.Unix(),
	}
	ordered := router.requestCandidates(accounts, selection, now)
	if len(ordered) != 2 || ordered[0].ID != "account-a" {
		t.Fatalf("health current order = %v, want healthy account-a first", []string{ordered[0].ID, ordered[1].ID})
	}
}

func TestProdex04355PreferredCurrentYieldsToPersistedQuotaOnResponses(t *testing.T) {
	now := time.Unix(250_000, 0)
	pressure := pressureScore(0, 100)
	quota := &cachedPressureQuota{byID: map[string]quotamodel.Availability{
		"account-a": {Ready: true, Pressure: pressure, Source: quotamodel.SourceLive},
		"account-b": {Ready: true, Pressure: pressure, Source: quotamodel.SourcePersistedSnapshot},
	}}
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true, RouteOrder: 2},
		{ID: "account-b", Home: "/b", Enabled: true, RouteOrder: 1},
	}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, PreferredAccount: "account-b", QuotaPreflight: quota,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ordered := router.requestCandidates(accounts, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, now)
	if len(ordered) != 2 || ordered[0].ID != "account-a" {
		t.Fatalf("persisted current order = %v, want live account-a first", []string{ordered[0].ID, ordered[1].ID})
	}
}
