package routing

import (
	"context"
	"net/http"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestRetryableResponseBackoffSurvivesRouterRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0)
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true},
	}
	accountSource := func(context.Context) ([]proxymodel.Account, error) {
		return append([]proxymodel.Account(nil), accounts...), nil
	}
	firstGateway := &routeHealthGateway{failing: "account-a"}
	firstRouter, err := NewRouter(Config{
		Gateway: firstGateway, PreferredAccount: "account-a", RoutingState: store,
		Now: func() time.Time { return now }, Accounts: accountSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstRouter.Forward(ctx, proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	if first.Result.AccountID != "account-b" {
		t.Fatalf("initial retry owner = %q, want account-b", first.Result.AccountID)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	backoffs, err := store.LoadRetryBackoffs(ctx, now)
	if err != nil || len(backoffs) != 1 || backoffs[0].AccountID != "account-a" ||
		backoffs[0].UntilUnix != now.Add(defaultProfileRetryBackoff).Unix() {
		t.Fatalf("persisted retry backoffs = %+v, error = %v", backoffs, err)
	}

	secondGateway := &pressureGateway{}
	secondRouter, err := NewRouter(Config{
		Gateway: secondGateway, PreferredAccount: "account-a", RoutingState: routingrepo.NewStore(root),
		Now: func() time.Time { return now }, Accounts: accountSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondRouter.Forward(ctx, proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Result.AccountID != "account-b" || len(secondGateway.owners) != 1 || secondGateway.owners[0] != "account-b" {
		t.Fatalf("post-restart retry owner = %q, requests = %v", second.Result.AccountID, secondGateway.owners)
	}
}

func TestSuccessfulResponseClearsRetryBackoff(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0)
	start := now
	root := t.TempDir()
	store := routingrepo.NewStore(root)
	if err := store.SetRetryBackoff(ctx, routingentity.RetryBackoff{
		AccountID: "account-a", UntilUnix: now.Add(time.Minute).Unix(),
	}, now); err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(Config{
		Gateway: &pressureGateway{}, PreferredAccount: "account-a", RoutingState: store,
		Now:  func() time.Time { return now },
		Wait: func(_ context.Context, delay time.Duration) error { now = now.Add(delay); return nil },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(ctx, proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	if err := exchange.Close(); err != nil {
		t.Fatal(err)
	}
	backoffs, err := store.LoadRetryBackoffs(ctx, start)
	if err != nil || len(backoffs) != 0 {
		t.Fatalf("retry backoffs after success = %+v, error = %v", backoffs, err)
	}
}
