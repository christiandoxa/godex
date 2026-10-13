package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type watchdogGateway04378 struct {
	calls               atomic.Int32
	cancelled           atomic.Bool
	servedContext       chan context.Context
	delay               time.Duration
	waitForCancellation bool
}

func (gateway *watchdogGateway04378) Execute(ctx context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	gateway.calls.Add(1)
	if gateway.servedContext != nil {
		select {
		case gateway.servedContext <- ctx:
		default:
		}
	}
	if gateway.waitForCancellation {
		<-ctx.Done()
		gateway.cancelled.Store(true)
		return nil, ctx.Err()
	}
	if gateway.delay > 0 {
		timer := time.NewTimer(gateway.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			gateway.cancelled.Store(true)
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader("{}")),
	}, nil
}

func watchdogRouter04378(t *testing.T, gateway *watchdogGateway04378, budget time.Duration) *Router {
	t.Helper()
	router, err := NewRouter(Config{
		Gateway:                 gateway,
		PrecommitWatchdogBudget: budget,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "watchdog-a", Home: "/synthetic/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func watchdogRequest04378(path string) proxymodel.Request {
	return proxymodel.Request{
		Method: http.MethodPost, Path: path, Header: make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}
}

func TestProdex04378PrecommitWatchdogReturns504AndCancelsUpstream(t *testing.T) {
	gateway := &watchdogGateway04378{waitForCancellation: true}
	router := watchdogRouter04378(t, gateway, 45*time.Millisecond)
	started := time.Now()
	exchange, err := router.Forward(context.Background(), watchdogRequest04378("/backend-api/godex/responses"))
	if exchange != nil {
		_ = exchange.Close()
		t.Fatal("deadline unexpectedly committed a response")
	}
	var presented *proxymodel.Error
	if !errors.As(err, &presented) || presented.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("precommit timeout=%v, want typed HTTP 504", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("watchdog exceeded bounded deadline: %s", elapsed)
	}
	if gateway.calls.Load() != 1 || !gateway.cancelled.Load() {
		t.Fatalf("pending transport wasn't canceled: calls=%d cancelled=%t", gateway.calls.Load(), gateway.cancelled.Load())
	}
}

func TestProdex04378WatchdogEndsAtPrecommitInsteadOfKillingStream(t *testing.T) {
	contexts := make(chan context.Context, 1)
	gateway := &watchdogGateway04378{servedContext: contexts}
	router := watchdogRouter04378(t, gateway, 35*time.Millisecond)
	exchange, err := router.Forward(context.Background(), watchdogRequest04378("/backend-api/godex/responses"))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	var upstream context.Context
	select {
	case upstream = <-contexts:
	default:
		t.Fatal("no upstream attempt observed")
	}
	time.Sleep(65 * time.Millisecond)
	if upstream.Err() != nil {
		t.Fatalf("committed response context expired after precommit: %v", upstream.Err())
	}
	data, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil || string(append(exchange.Result.Prefix, data...)) != "{}" {
		t.Fatalf("committed response prefix=%q body=%q error=%v", exchange.Result.Prefix, data, err)
	}
}

func TestProdex04378WatchdogDoesNotApplyToOrdinaryMetadata(t *testing.T) {
	gateway := &watchdogGateway04378{delay: 70 * time.Millisecond}
	router := watchdogRouter04378(t, gateway, 30*time.Millisecond)
	exchange, err := router.Forward(context.Background(), watchdogRequest04378("/backend-api/godex/models"))
	if err != nil {
		t.Fatalf("ordinary metadata was prematurely timed out: %v", err)
	}
	defer exchange.Close()
	if gateway.cancelled.Load() {
		t.Fatal("ordinary metadata transport was canceled")
	}
}

func TestProdex04378ExplicitClientCancellationIsNotLocal504(t *testing.T) {
	gateway := &watchdogGateway04378{waitForCancellation: true}
	router := watchdogRouter04378(t, gateway, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(25 * time.Millisecond); cancel() }()
	exchange, err := router.Forward(ctx, watchdogRequest04378("/backend-api/godex/responses"))
	if exchange != nil {
		_ = exchange.Close()
		t.Fatal("cancelled request committed a response")
	}
	var presented *proxymodel.Error
	if errors.As(err, &presented) && presented.StatusCode == http.StatusGatewayTimeout {
		t.Fatalf("client cancellation was mislabeled as watchdog: %v", err)
	}
	if !gateway.cancelled.Load() {
		t.Fatal("client cancellation did not reach upstream")
	}
}
