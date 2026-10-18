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

// An intentionally context-insensitive fake gateway exposes unauthorized
// retry attempts that must be blocked by the Router's ORIGINAL deadline,
// even if the first transport result itself arrives after cancellation.
type wsCanceledRetryGateway04382 struct {
	attempts     atomic.Int32
	closed       atomic.Int32
	cancel       context.CancelFunc
	firstStatus  int
	firstFailure *proxymodel.PrecommitFailure
}

func (g *wsCanceledRetryGateway04382) ExecuteWebSocket(_ context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	return g.firstResult()
}
func (g *wsCanceledRetryGateway04382) ExecuteWebSocketMessage(_ context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	return g.firstResult()
}
func (g *wsCanceledRetryGateway04382) firstResult() (*proxymodel.Response, error) {
	count := g.attempts.Add(1)
	if count == 1 && g.cancel != nil {
		g.cancel()
	}
	status := g.firstStatus
	if count != 1 {
		status = http.StatusOK
	}
	return &proxymodel.Response{
		StatusCode:      status,
		Body:            &wsCanceledRetryBody04382{ReadCloser: io.NopCloser(strings.NewReader("synthetic")), closed: &g.closed},
		WebSocketFrames: true, WebSocketTurnState: "turn-owner",
		PrecommitFailure: g.firstFailure,
	}, nil
}

type wsCanceledRetryBody04382 struct {
	io.ReadCloser
	closed *atomic.Int32
}

func (b *wsCanceledRetryBody04382) Close() error { b.closed.Add(1); return b.ReadCloser.Close() }

func TestProdex04382CanceledWebSocketAuthReloadNeverStartsSecondAttempt(t *testing.T) {
	for _, kind := range []string{"message", "upgrade"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			gateway := &wsCanceledRetryGateway04382{cancel: cancel, firstStatus: http.StatusUnauthorized}
			router := &Router{}
			request := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
			var response *proxymodel.Response
			var err error
			if kind == "message" {
				request.WebSocketMessage = true
				response, err = router.executeWebSocketMessage(ctx, request, proxymodel.Account{ID: "synthetic-owner"}, gateway)
			} else {
				request.Header.Set("Upgrade", "websocket")
				response, err = router.executeWebSocket(ctx, request, proxymodel.Account{ID: "synthetic-owner"}, gateway)
			}
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			if response != nil || !errors.Is(err, context.Canceled) || gateway.attempts.Load() != 1 || gateway.closed.Load() != 1 {
				t.Fatalf("%s renewed expired auth attempt: response=%+v err=%v attempts=%d closes=%d",
					kind, response, err, gateway.attempts.Load(), gateway.closed.Load())
			}
		})
	}
}

func TestProdex04382CanceledWebSocketOwnerTransportRetryNeverReplays(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gateway := &wsCanceledRetryGateway04382{
		cancel: cancel, firstStatus: http.StatusBadGateway,
		firstFailure: &proxymodel.PrecommitFailure{Code: "connect_timeout", Transport: true},
	}
	router := &Router{}
	request := proxymodel.Request{
		Method: http.MethodPost, Path: "/responses", Header: make(http.Header), WebSocketMessage: true,
		Body: []byte(`{"type":"response.create","previous_response_id":"resp-native"}`),
	}
	response, err := router.executeWebSocketMessage(ctx, request, proxymodel.Account{ID: "synthetic-owner"}, gateway)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if response != nil || !errors.Is(err, context.Canceled) || gateway.attempts.Load() != 1 || gateway.closed.Load() != 1 {
		t.Fatalf("expired bound owner transport retried: response=%+v err=%v attempts=%d closes=%d",
			response, err, gateway.attempts.Load(), gateway.closed.Load())
	}
}

func TestProdex04382AlreadyExpiredWebSocketNeverStartsInitialAttempt(t *testing.T) {
	gateway := &wsCanceledRetryGateway04382{firstStatus: http.StatusOK}
	router := &Router{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header), WebSocketMessage: true}
	response, err := router.executeWebSocketMessage(ctx, request, proxymodel.Account{ID: "synthetic-owner"}, gateway)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if response != nil || !errors.Is(err, context.Canceled) || gateway.attempts.Load() != 0 {
		t.Fatalf("expired WebSocket began inference: response=%+v err=%v attempts=%d", response, err, gateway.attempts.Load())
	}
}

func TestProdex04382ExpiredWebSocketDeadlineNeverStartsHandshakeOrMessage(t *testing.T) {
	for _, kind := range []string{"message", "upgrade"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			gateway := &wsCanceledRetryGateway04382{firstStatus: http.StatusUnauthorized}
			router := &Router{}
			request := proxymodel.Request{Method: http.MethodPost, Path: "/responses", Header: make(http.Header)}
			var response *proxymodel.Response
			var err error
			if kind == "message" {
				request.WebSocketMessage = true
				response, err = router.executeWebSocketMessage(ctx, request, proxymodel.Account{ID: "synthetic-owner"}, gateway)
			} else {
				request.Header.Set("Upgrade", "websocket")
				response, err = router.executeWebSocket(ctx, request, proxymodel.Account{ID: "synthetic-owner"}, gateway)
			}
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			if response != nil || !errors.Is(err, context.DeadlineExceeded) || gateway.attempts.Load() != 0 {
				t.Fatalf("expired %s deadline was renewed on upstream: response=%+v err=%v calls=%d",
					kind, response, err, gateway.attempts.Load())
			}
		})
	}
}
