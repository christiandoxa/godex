package openai

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

// Prodex 0.438.2: a completed Responses WebSocket retains a reusable socket
// and response lineage but releases its weighted profile execution permit.
// A second active turn can saturate the only ready profile; reuse must wait
// without evicting the first session, then reacquire exactly one permit.
func TestProdex04382CompletedIdleWebSocketReleasesPermitAndReuseWaits(t *testing.T) {
	var handshakes, upstreamCalls atomic.Int32
	server := newWebSocketMessageParityServer(t, nil, func(conn net.Conn, reader *bufio.Reader, _ *http.Request) {
		handshakes.Add(1)
		for {
			_, err := readWebSocketParityRequest(reader)
			if err != nil {
				return
			}
			count := upstreamCalls.Add(1)
			frame := []byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp-%d"}}`, count))
			if writeWebSocketParityEvent(conn, frame) != nil {
				return
			}
		}
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	account := proxymodel.Account{ID: "ready-profile", Home: "synthetic-home", Enabled: true}
	waiting := make(chan struct{}, 1)
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: transport, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil },
		ProfileInflightWait: func(ctx context.Context, changed <-chan struct{}, _ time.Duration) error {
			select {
			case waiting <- struct{}{}:
			default:
			}
			select {
			case <-changed:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	makeRequest := func(session uint64) proxymodel.Request {
		r := websocketParityRequest(`{"type":"response.create"}`, session, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true})
		r.QuotaSelection = quotamodel.Selection{RouteKind: quotamodel.RouteKindWebSocket}
		return r
	}
	drain := func(exchange *routingusecase.Exchange) { // `Exchange` is owned by router; drain before closing.
		t.Helper()
		data, readErr := io.ReadAll(io.LimitReader(exchange.Result.Response.Body, 8192))
		closeErr := exchange.Close()
		if readErr != nil || closeErr != nil || !strings.Contains(string(data), "response.completed") {
			t.Fatalf("completed WebSocket read/close: body=%q read=%v close=%v", data, readErr, closeErr)
		}
	}
	first, err := router.Forward(context.Background(), makeRequest(101))
	if err != nil {
		t.Fatal(err)
	}
	drain(first)
	transport.websocketMessageMu.Lock()
	_, cached := transport.websocketMessageSessions[101]
	transport.websocketMessageMu.Unlock()
	if !cached {
		t.Fatal("completed Responses upstream socket was not cached")
	}
	// With the first completed socket idle, a second session must be able
	// to start its own active response on the same weighted capacity=2.
	second, err := router.Forward(context.Background(), makeRequest(202))
	if err != nil {
		t.Fatalf("idle completed socket monopolized the ready profile: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	completed := make(chan struct {
		exchange *routingusecase.Exchange
		err      error
	}, 1)
	go func() {
		e, eErr := router.Forward(ctx, makeRequest(101))
		completed <- struct {
			exchange *routingusecase.Exchange
			err      error
		}{e, eErr}
	}()
	select {
	case <-waiting:
		if calls := upstreamCalls.Load(); calls != 2 {
			t.Fatalf("saturated reuse dispatched too early: upstream calls=%d", calls)
		}
	case done := <-completed:
		if done.exchange != nil {
			_ = done.exchange.Close()
		}
		t.Fatalf("saturated reuse returned before permit release: %v", done.err)
	case <-ctx.Done():
		t.Fatal("reused idle WebSocket did not reach capacity waiter")
	}
	transport.websocketMessageMu.Lock()
	_, cached = transport.websocketMessageSessions[101]
	transport.websocketMessageMu.Unlock()
	if !cached {
		t.Fatal("admission wait evicted the cached upstream connection")
	}
	drain(second)
	select {
	case done := <-completed:
		if done.err != nil {
			t.Fatalf("reuse after release: %v", done.err)
		}
		drain(done.exchange)
	case <-ctx.Done():
		t.Fatal("reused session did not resume after permit release")
	}
	if handshakes.Load() != 2 || upstreamCalls.Load() != 3 {
		t.Fatalf("idle/reused sockets: handshakes=%d (want 2), upstream=%d (want 3)", handshakes.Load(), upstreamCalls.Load())
	}
}
