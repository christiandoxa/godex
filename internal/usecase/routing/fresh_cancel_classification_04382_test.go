package routing

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// The 0.438.2 request-scoped watchdog distinguishes local cancellation from
// actual per-profile capacity exhaustion. An expired fresh request must not
// be put back into the saturated-profile admission queue or start upstream.
func TestProdex04382ExpiredFreshAttemptIsNotCapacitySaturation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		want    error
	}{
		{"canceled", func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, context.Canceled},
		{"expired deadline", func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}, context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &canceledAdmissionGateway04382{}
			router, err := NewRouter(Config{
				Gateway: gateway, ProfileInflightHardLimit: 2,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			ctx, cancel := tc.context()
			defer cancel()
			request := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
			forwarded, pending, saturated, err := router.freshAttempt(ctx, request, proxymodel.Account{ID: "ready"}, false)
			if forwarded != nil || pending != nil || saturated || !errors.Is(err, tc.want) {
				t.Fatalf("expired fresh request classified as capacity: forwarded=%+v pending=%+v saturated=%t err=%v", forwarded, pending, saturated, err)
			}
			if gateway.calls.Load() != 0 || router.profileInflightAdmissionsTotal != 0 || router.inflight["ready"] != 0 {
				t.Fatalf("expired request dispatched or took permit: calls=%d admissions=%d capacity=%d", gateway.calls.Load(), router.profileInflightAdmissionsTotal, router.inflight["ready"])
			}
		})
	}
}

func TestProdex04382SaturatedFreshAttemptStillReportsOnlyCapacity(t *testing.T) {
	gateway := &canceledAdmissionGateway04382{}
	router, err := NewRouter(Config{
		Gateway: gateway, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	request := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
	held, ok := router.tryAcquireProfileInflight("ready", request, false)
	if !ok {
		t.Fatal("unable to establish real capacity saturation")
	}
	defer held()
	forwarded, pending, saturated, err := router.freshAttempt(context.Background(), request, proxymodel.Account{ID: "ready"}, false)
	if forwarded != nil || pending != nil || !saturated || err != nil || gateway.calls.Load() != 0 {
		t.Fatalf("capacity saturation misclassified: forwarded=%+v pending=%+v saturated=%t err=%v calls=%d", forwarded, pending, saturated, err, gateway.calls.Load())
	}
}
