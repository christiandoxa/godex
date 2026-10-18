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
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

// An intentionally context-insensitive synthetic gateway is an adversarial
// negative control: once the request deadline/cancellation has expired,
// routing MUST stop admission rather than relying on the network adapter
// to notice a canceled context before sending inference bytes.
type canceledAdmissionGateway04382 struct{ calls atomic.Int32 }

func (g *canceledAdmissionGateway04382) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	g.calls.Add(1)
	return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
}

type cancelOnInflightMarker04382 struct{ cancel context.CancelFunc }

func (recorder cancelOnInflightMarker04382) Record(_ context.Context, event runtimemodel.Event) error {
	if event.Kind == "profile_inflight" && event.Fields["count"] == "2" {
		recorder.cancel()
	}
	return nil
}

func TestProdex04382ExpiredRequestCannotAcquireFreshProfilePermit(t *testing.T) {
	for _, hardAffinity := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "hard-affinity"}[hardAffinity], func(t *testing.T) {
			gateway := &canceledAdmissionGateway04382{}
			router, err := NewRouter(Config{
				Gateway: gateway, ProfileInflightHardLimit: 2,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			req := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
			response, acquired, err := router.tryExecuteWithProfileInflight(ctx, req, proxymodel.Account{ID: "ready"}, hardAffinity)
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			if !errors.Is(err, context.Canceled) || acquired || response != nil {
				t.Fatalf("expired request admitted despite cancellation: acquired=%t response=%+v err=%v", acquired, response, err)
			}
			if gateway.calls.Load() != 0 {
				t.Fatalf("expired request sent %d upstream requests", gateway.calls.Load())
			}
			if got := router.inflight["ready"]; got != 0 {
				t.Fatalf("canceled request kept %d permits", got)
			}
			if router.profileInflightAdmissionsTotal != 0 {
				t.Fatal("expired request acquired weighted profile admission")
			}
		})
	}
}

func TestProdex04382CanceledWaiterAwakenedByPermitReleaseCannotDispatch(t *testing.T) {
	gateway := &canceledAdmissionGateway04382{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var held *proxymodel.Response
	var waiterCalls int
	router, err := NewRouter(Config{
		Gateway: gateway, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
		ProfileInflightWait: func(_ context.Context, _ <-chan struct{}, _ time.Duration) error {
			waiterCalls++
			cancel()
			if held != nil {
				_ = held.Body.Close()
			} // release races expired waiter wakeup
			return nil // release notification arrives despite canceled context
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	req := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
	account := proxymodel.Account{ID: "ready"}
	first, acquired, err := router.tryExecuteWithProfileInflight(context.Background(), req, account, false)
	if err != nil || !acquired || first == nil {
		t.Fatalf("could not hold profile capacity: %v", err)
	}
	held = first
	defer held.Body.Close()
	result, err := router.executeWithProfileInflightWait(ctx, req, account, false)
	if result != nil && result.Body != nil {
		_ = result.Body.Close()
	}
	if !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("awakened canceled waiter dispatched: response=%+v err=%v", result, err)
	}
	if waiterCalls != 1 || gateway.calls.Load() != 1 {
		t.Fatalf("canceled waiter performed inference replay: waits=%d gateway calls=%d", waiterCalls, gateway.calls.Load())
	}
	if router.inflight["ready"] != 0 || router.profileInflightReleaseUnderflowsTotal != 0 {
		t.Fatalf("permit release leaked or underflowed: %+v", router.inflight)
	}
}

func TestProdex04382CancellationDuringPermitAdmissionBlocksUpstreamSend(t *testing.T) {
	gateway := &canceledAdmissionGateway04382{}
	ctx, cancel := context.WithCancel(context.Background())
	router, err := NewRouter(Config{
		Gateway: gateway, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
		Activity: cancelOnInflightMarker04382{cancel: cancel},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	req := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
	result, acquired, err := router.tryExecuteWithProfileInflight(ctx, req, proxymodel.Account{ID: "ready"}, false)
	if result != nil && result.Body != nil {
		_ = result.Body.Close()
	}
	if !acquired || result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled after admission marker still reached upstream: acquired=%t response=%+v err=%v", acquired, result, err)
	}
	if gateway.calls.Load() != 0 || router.inflight["ready"] != 0 ||
		router.profileInflightAdmissionsTotal != 1 || router.profileInflightReleasesTotal != 1 ||
		router.profileInflightReleaseUnderflowsTotal != 0 {
		t.Fatalf("post-admission cancellation: gateway=%d admission=%d release=%d underflow=%d",
			gateway.calls.Load(), router.profileInflightAdmissionsTotal, router.profileInflightReleasesTotal, router.profileInflightReleaseUnderflowsTotal)
	}
}

func TestProdex04382ExpiredDeadlineCannotDispatchBoundOrFreshTurn(t *testing.T) {
	gateway := &canceledAdmissionGateway04382{}
	router, err := NewRouter(Config{
		Gateway: gateway, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	req := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
	account := proxymodel.Account{ID: "ready"}
	for _, bound := range []bool{false, true} {
		response, acquired, err := router.tryExecuteWithProfileInflight(ctx, req, account, bound)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if response != nil || acquired || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline-expired bound=%t: response=%+v acquired=%t err=%v", bound, response, acquired, err)
		}
	}
	if gateway.calls.Load() != 0 || router.inflight["ready"] != 0 ||
		router.profileInflightAdmissionsTotal != 0 || router.profileInflightReleasesTotal != 0 {
		t.Fatalf("expired request counted/admitted upstream: calls=%d admits=%d releases=%d",
			gateway.calls.Load(), router.profileInflightAdmissionsTotal, router.profileInflightReleasesTotal)
	}
}
