package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

const (
	retryBackoffAccountA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	retryBackoffAccountB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type retryBackoffGateway struct {
	failAccount string
	owners      []string
}

func (gateway *retryBackoffGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.owners = append(gateway.owners, account.ID)
	status := http.StatusOK
	body := "ok"
	if account.ID == gateway.failAccount {
		status = http.StatusServiceUnavailable
		body = "unavailable"
	}
	return &proxymodel.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func retryBackoffAccounts() []proxymodel.Account {
	return []proxymodel.Account{
		{ID: retryBackoffAccountA, Home: "/a", Enabled: true},
		{ID: retryBackoffAccountB, Home: "/b", Enabled: true},
	}
}

func TestProdex04355RetryableResponseBackoffSurvivesRouterRestart(t *testing.T) {
	now := time.Unix(50_000, 0)
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	accounts := retryBackoffAccounts()
	source := func(context.Context) ([]proxymodel.Account, error) {
		return append([]proxymodel.Account(nil), accounts...), nil
	}

	firstGateway := &retryBackoffGateway{failAccount: retryBackoffAccountA}
	firstRouter, err := NewRouter(Config{
		Gateway: firstGateway, Accounts: source, PreferredAccount: retryBackoffAccountA,
		RoutingState: store, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstRouter.Forward(t.Context(), proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	if first.Result.AccountID != retryBackoffAccountB {
		t.Fatalf("first result owner = %q", first.Result.AccountID)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	backoffs, err := store.LoadRetryBackoffs(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	wantUntil := now.Unix() + 20
	if len(backoffs) != 1 || backoffs[0].AccountID != retryBackoffAccountA || backoffs[0].UntilUnix != wantUntil {
		t.Fatalf("persisted backoff = %+v, want account A until %d", backoffs, wantUntil)
	}

	secondGateway := &retryBackoffGateway{}
	secondRouter, err := NewRouter(Config{
		Gateway: secondGateway, Accounts: source, PreferredAccount: retryBackoffAccountA,
		RoutingState: routingrepo.NewStore(root), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondRouter.Forward(t.Context(), proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Result.AccountID != retryBackoffAccountB || len(secondGateway.owners) != 1 || secondGateway.owners[0] != retryBackoffAccountB {
		t.Fatalf("restart routing = owner %q attempts %v", second.Result.AccountID, secondGateway.owners)
	}
}

// Prodex 0.436.1 rechecks a fresh transient API-key pool after restart,
// even when a primary key got a rate limit during the preceding process.
// The managed-profile test above intentionally asserts the opposite lifetime.
func TestProdex04361EphemeralAPIKeyBackoffDoesNotSurviveRestart(t *testing.T) {
	now := time.Unix(60_000, 0)
	store := routingrepo.NewStore(t.TempDir())
	accounts := retryBackoffAccounts()
	for index := range accounts {
		accounts[index].Provider.Kind = "deepseek"
		accounts[index].EphemeralAPIKey = true
	}
	source := func(context.Context) ([]proxymodel.Account, error) {
		return append([]proxymodel.Account(nil), accounts...), nil
	}
	launch := func() *retryBackoffGateway {
		gateway := &retryBackoffGateway{failAccount: retryBackoffAccountA}
		router, err := NewRouter(Config{
			Gateway: gateway, Accounts: source, PreferredAccount: retryBackoffAccountA,
			RoutingState: store, Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		exchange, err := router.Forward(t.Context(), proxymodel.Request{
			Header:         make(http.Header),
			QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
		})
		if err != nil {
			t.Fatal(err)
		}
		if exchange.Result.Response.StatusCode != http.StatusOK {
			t.Fatalf("rotated response status = %d", exchange.Result.Response.StatusCode)
		}
		if err := exchange.Close(); err != nil {
			t.Fatal(err)
		}
		return gateway
	}
	first := launch()
	if got := strings.Join(first.owners, ","); got != retryBackoffAccountA+","+retryBackoffAccountB {
		t.Fatalf("first launch: ordered upstream attempts %q", got)
	}
	backoffs, err := store.LoadRetryBackoffs(t.Context(), now)
	if err != nil || len(backoffs) != 0 {
		t.Fatalf("transient key backoff persisted: %v, err=%v", backoffs, err)
	}
	scores, err := store.LoadRouteHealth(t.Context(), now)
	if err != nil || len(scores) != 0 {
		t.Fatalf("transient key health score persisted: %+v, err=%v", scores, err)
	}
	second := launch()
	if got := strings.Join(second.owners, ","); got != retryBackoffAccountA+","+retryBackoffAccountB {
		t.Fatalf("restart changed transient key ordering: %q", got)
	}
}

func TestProdex04361TransientPoolKeepsInProcessBackoff(t *testing.T) {
	now := time.Unix(70_000, 0)
	store := routingrepo.NewStore(t.TempDir())
	accounts := retryBackoffAccounts()
	for i := range accounts {
		accounts[i].EphemeralAPIKey = true
		accounts[i].Provider.Kind = "deepseek"
	}
	gateway := &retryBackoffGateway{failAccount: retryBackoffAccountA}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: retryBackoffAccountA,
		Accounts:     func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
		RoutingState: store, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	for turn := 0; turn < 2; turn++ {
		exchange, err := router.Forward(t.Context(), proxymodel.Request{
			Header:         make(http.Header),
			QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
		})
		if err != nil {
			t.Fatalf("turn %d: %v", turn, err)
		}
		if exchange.Result.Response.StatusCode != http.StatusOK {
			t.Fatalf("turn %d status %d", turn, exchange.Result.Response.StatusCode)
		}
		if err := exchange.Close(); err != nil {
			t.Fatal(err)
		}
	}
	want := retryBackoffAccountA + "," + retryBackoffAccountB + "," + retryBackoffAccountB
	if got := strings.Join(gateway.owners, ","); got != want {
		t.Fatalf("in-process cooldown not enforced: got %s, want %s", got, want)
	}
	backoffs, err := store.LoadRetryBackoffs(t.Context(), now)
	if err != nil || len(backoffs) != 0 {
		t.Fatalf("ephemeral backoff persisted: %v, %v", backoffs, err)
	}
}

func TestProdex04361ManagedHealthPenaltyIsStillDurable(t *testing.T) {
	now := time.Unix(80_000, 0)
	store := routingrepo.NewStore(t.TempDir())
	accounts := retryBackoffAccounts()
	gateway := &retryBackoffGateway{failAccount: retryBackoffAccountA}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: retryBackoffAccountA,
		Accounts:     func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
		RoutingState: store, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Header:         make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := exchange.Close(); err != nil {
		t.Fatal(err)
	}
	scores, err := store.LoadRouteHealth(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, score := range scores {
		if score.AccountID == retryBackoffAccountA && score.Score > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("managed profile health penalty lost: %v", scores)
	}
	backoffs, err := store.LoadRetryBackoffs(t.Context(), now)
	if err != nil || len(backoffs) != 1 || backoffs[0].AccountID != retryBackoffAccountA {
		t.Fatalf("managed profile retry backoff lost: %v, %v", backoffs, err)
	}
}

func TestProdex04355SuccessfulResponseClearsPersistedRetryBackoff(t *testing.T) {
	start := time.Unix(60_000, 0)
	now := start
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	if err := store.SetRetryBackoff(t.Context(), routingentity.RetryBackoff{
		AccountID: retryBackoffAccountA, UntilUnix: start.Unix() + 1,
	}, start); err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(Config{
		Gateway: &retryBackoffGateway{}, PreferredAccount: retryBackoffAccountA,
		RoutingState: store, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: retryBackoffAccountA, Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	now = start.Add(2 * time.Second)
	exchange, err := router.Forward(t.Context(), proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	if err := exchange.Close(); err != nil {
		t.Fatal(err)
	}
	backoffs, err := store.LoadRetryBackoffs(t.Context(), start)
	if err != nil || len(backoffs) != 0 {
		t.Fatalf("backoffs after success = %+v err=%v", backoffs, err)
	}
}

func TestProdex04355RetryBackoffRoundsDurationUpToWholeSecond(t *testing.T) {
	now := time.Unix(70_000, 0)
	store := routingrepo.NewStore(t.TempDir())
	router, err := NewRouter(Config{
		Gateway: &retryBackoffGateway{}, RoutingState: store, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return retryBackoffAccounts(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	router.persistRetryBackoff(t.Context(), retryBackoffAccountA, 1100*time.Millisecond)
	backoffs, err := store.LoadRetryBackoffs(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	wantUntil := now.Unix() + 2
	if len(backoffs) != 1 || backoffs[0].UntilUnix != wantUntil {
		t.Fatalf("rounded backoff = %+v, want until %d", backoffs, wantUntil)
	}
}
