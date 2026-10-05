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
