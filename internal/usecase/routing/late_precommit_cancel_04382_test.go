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
)

// An uncooperative provider returns a fully-formed response *after* the
// request was canceled. The Router must close noncommitted output before any
// downstream delivery/quota classification and release weighted capacity.
// Committed output has a different lifetime from precommit watchdog expiry.
type latePrecommitGateway04382 struct {
	cancel      context.CancelFunc
	calls       atomic.Int32
	closed      atomic.Int32
	committed   bool
	awaitExpiry bool
}

func (g *latePrecommitGateway04382) Execute(ctx context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	g.calls.Add(1)
	if g.awaitExpiry {
		<-ctx.Done()
	}
	if g.cancel != nil {
		g.cancel()
	}
	return &proxymodel.Response{
		StatusCode:          http.StatusOK,
		FirstEventCommitted: g.committed,
		Body:                &latePrecommitBody04382{ReadCloser: io.NopCloser(strings.NewReader("synthetic-output")), closed: &g.closed},
	}, nil
}

type latePrecommitBody04382 struct {
	io.ReadCloser
	closed *atomic.Int32
}

func (b *latePrecommitBody04382) Close() error { b.closed.Add(1); return b.ReadCloser.Close() }

func TestProdex04382LateUncommittedResultAfterCancelCannotBeDelivered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gateway := &latePrecommitGateway04382{cancel: cancel}
	router, err := NewRouter(Config{
		Gateway: gateway, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	request := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
	response, acquired, err := router.tryExecuteWithProfileInflight(ctx, request, proxymodel.Account{ID: "ready"}, false)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if !acquired || response != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("late precommit result was delivered despite cancellation: response=%+v acquired=%t err=%v", response, acquired, err)
	}
	if gateway.calls.Load() != 1 || gateway.closed.Load() != 1 ||
		router.inflight["ready"] != 0 || router.profileInflightAdmissionsTotal != 1 ||
		router.profileInflightReleasesTotal != 1 || router.profileInflightReleaseUnderflowsTotal != 0 {
		t.Fatalf("late precommit lifecycle dispatch/close/admission/release/underflow=%d/%d/%d/%d/%d",
			gateway.calls.Load(), gateway.closed.Load(), router.profileInflightAdmissionsTotal, router.profileInflightReleasesTotal, router.profileInflightReleaseUnderflowsTotal)
	}
}

func TestProdex04382CommittedResultRetainsPermitPastOriginalDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gateway := &latePrecommitGateway04382{cancel: cancel, committed: true}
	router, err := NewRouter(Config{
		Gateway: gateway, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	request := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
	response, acquired, err := router.tryExecuteWithProfileInflight(ctx, request, proxymodel.Account{ID: "ready"}, false)
	if err != nil || !acquired || response == nil || !response.FirstEventCommitted {
		t.Fatalf("already committed output was incorrectly canceled: response=%+v acquired=%t err=%v", response, acquired, err)
	}
	if router.inflight["ready"] != 2 || gateway.closed.Load() != 0 {
		t.Fatalf("committed stream released permit/body before output drain")
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if gateway.calls.Load() != 1 || gateway.closed.Load() != 1 || router.inflight["ready"] != 0 ||
		router.profileInflightReleasesTotal != 1 || router.profileInflightReleaseUnderflowsTotal != 0 {
		t.Fatalf("committed cleanup calls=%d closes=%d releases=%d underflow=%d",
			gateway.calls.Load(), gateway.closed.Load(), router.profileInflightReleasesTotal, router.profileInflightReleaseUnderflowsTotal)
	}
}

func TestProdex04382LatePrecommitDeadlineExpiresBeforeGatewayResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()
	gateway := &latePrecommitGateway04382{awaitExpiry: true}
	router, err := NewRouter(Config{
		Gateway: gateway, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	req := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
	resp, acquired, err := router.tryExecuteWithProfileInflight(ctx, req, proxymodel.Account{ID: "ready"}, false)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if !acquired || resp != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("late precommit success renewed original deadline: acquired=%t resp=%+v err=%v", acquired, resp, err)
	}
	if gateway.calls.Load() != 1 || gateway.closed.Load() != 1 ||
		router.inflight["ready"] != 0 || router.profileInflightReleasesTotal != 1 ||
		router.profileInflightReleaseUnderflowsTotal != 0 {
		t.Fatalf("late precommit deadline leaked capacity: calls=%d close=%d releases=%d underflow=%d",
			gateway.calls.Load(), gateway.closed.Load(), router.profileInflightReleasesTotal, router.profileInflightReleaseUnderflowsTotal)
	}
}

func TestProdex04382FreshRequestAfterLatePrecommitCancellationStillRuns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gateway := &latePrecommitGateway04382{cancel: cancel}
	router, err := NewRouter(Config{
		Gateway: gateway, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	request := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
	account := proxymodel.Account{ID: "ready"}
	old, _, err := router.tryExecuteWithProfileInflight(ctx, request, account, false)
	if old != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled predecessor leaked response: %+v %v", old, err)
	}
	next, acquired, err := router.tryExecuteWithProfileInflight(context.Background(), request, account, false)
	if err != nil || !acquired || next == nil || next.Body == nil {
		t.Fatalf("fresh request following late cancellation blocked: acquired=%t next=%+v err=%v", acquired, next, err)
	}
	if err := next.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if gateway.calls.Load() != 2 || gateway.closed.Load() != 2 ||
		router.profileInflightAdmissionsTotal != 2 || router.profileInflightReleasesTotal != 2 ||
		router.profileInflightReleaseUnderflowsTotal != 0 || router.inflight["ready"] != 0 {
		t.Fatalf("expired/fresh lifecycle calls=%d closed=%d admissions=%d releases=%d underflows=%d",
			gateway.calls.Load(), gateway.closed.Load(), router.profileInflightAdmissionsTotal, router.profileInflightReleasesTotal, router.profileInflightReleaseUnderflowsTotal)
	}
}
