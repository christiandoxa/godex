package proxy

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type committedDeadlineBody04383 struct {
	ctx    context.Context
	prefix io.Reader
	closed chan struct{}
	once   sync.Once
}

func (body *committedDeadlineBody04383) Read(data []byte) (int, error) {
	if count, err := body.prefix.Read(data); count != 0 || err != io.EOF {
		return count, err
	}
	select {
	case <-body.ctx.Done():
		return 0, body.ctx.Err()
	case <-body.closed:
		return 0, io.EOF
	}
}

func (body *committedDeadlineBody04383) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}

func TestProdex04383LocalWatchdogPreservesParentCancellationCause(t *testing.T) {
	parent, cancelParent := context.WithCancelCause(t.Context())
	ctx, cancel := withRequestDeadline(parent, time.Second)
	defer cancel()
	cancelParent(context.DeadlineExceeded)
	if ctx.Err() != context.Canceled || requestDeadlineTimedOut(ctx, ctx.Err()) {
		t.Fatal("parent cancellation cause was relabeled as the local watchdog")
	}
}

func TestProdex04383CommitKeepsTheParentsDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	parent, cancelParent := context.WithDeadline(t.Context(), deadline)
	defer cancelParent()
	ctx, cancel := withRequestDeadline(parent, time.Minute)
	defer cancel()
	disarmRequestDeadline(ctx)
	if got, present := ctx.Deadline(); !present || !got.Equal(deadline) {
		t.Fatal("disarming the local watchdog lost the parent's deadline")
	}
}

func TestProdex04383LocalWatchdogDoesNotChangeTheContextDeadlineAtCommit(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	parent, cancelParent := context.WithDeadline(t.Context(), deadline)
	defer cancelParent()
	ctx, cancel := withRequestDeadline(parent, time.Minute)
	defer cancel()
	before, presentBefore := ctx.Deadline()
	disarmRequestDeadline(ctx)
	after, presentAfter := ctx.Deadline()
	if !presentBefore || !presentAfter || !before.Equal(after) || !after.Equal(deadline) {
		t.Fatal("local watchdog violated the stable context deadline contract")
	}
}

func TestProdex04383CommittedHTTPStreamOutlivesLocalDeadlineAndKeepsClientCancellation(t *testing.T) {
	type attempt struct {
		ctx      context.Context
		deadline time.Time
	}
	started := make(chan attempt, 1)
	var calls atomic.Int32
	gateway := gatewayFunc(func(ctx context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
		calls.Add(1)
		scope, _ := requestDeadlineScopeFor(ctx)
		started <- attempt{ctx, scope.deadline}
		return &proxymodel.Response{
			StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body: &committedDeadlineBody04383{ctx: ctx, closed: make(chan struct{}), prefix: strings.NewReader("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-live\"}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"visible\"}\n\n")},
		}, nil
	})
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway, PrecommitWatchdogBudget: 150 * time.Millisecond,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "synthetic-a", Home: "/synthetic", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	proxy, err := NewProxy(Config{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	ctx, cancel := context.WithCancel(t.Context())
	defer func() { cancel(); server.Close() }()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/backend-api/godex/responses", strings.NewReader(`{"stream":true,"input":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, `"delta":"visible"`) {
			break
		}
	}
	upstream := <-started
	if response.StatusCode != 200 || upstream.deadline.IsZero() {
		t.Fatal("stream did not commit under the original request deadline")
	}
	pastDeadline := time.NewTimer(time.Until(upstream.deadline.Add(25 * time.Millisecond)))
	defer pastDeadline.Stop()
	select {
	case <-upstream.ctx.Done():
		t.Fatalf("local precommit deadline canceled committed HTTP output: %v", upstream.ctx.Err())
	case <-pastDeadline.C:
	}
	if _, present := upstream.ctx.Deadline(); present {
		t.Fatal("committed upstream still exposes the local precommit deadline")
	}
	cancel()
	select {
	case <-upstream.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("disarming the local deadline lost client cancellation")
	}
	if calls.Load() != 1 {
		t.Fatal("committed HTTP stream was replayed")
	}
}
