package openai

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Prodex 0.438.2 keeps the ORIGINAL precommit watchdog through hidden
// keepalive/metadata frames. The first text frame is not commitment; only
// actual model output transfers ownership to the committed idle timer.
func TestProdex04382HiddenWebSocketPreludeCannotPromoteIdleDeadline(t *testing.T) {
	account := proxymodel.Account{ID: "synthetic-a", Home: "synthetic-home"}
	transport, err := NewTransport("http://127.0.0.1", nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	client, upstream := net.Pipe()
	defer upstream.Close()
	watchdog := newWebSocketReadWatchdog(client, websocketPrecommitProgressTimeout)
	const session = uint64(438202)
	transport.recycleWebSocketMessageSession(session, account, watchdog, "")
	frameWritten := make(chan error, 1)
	go func() {
		if _, err := readWebSocketParityRequest(bufio.NewReader(upstream)); err != nil {
			frameWritten <- err
			return
		}
		frameWritten <- writeWebSocketParityEvent(upstream, []byte(`{"type":"keepalive","metadata":{"phase":"hidden"}}`))
		// A hidden prelude never completes the response; this peer stays
		// silent until cancellation closes the synthetic socket.
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		response, err := transport.ExecuteWebSocketMessage(ctx,
			websocketParityRequest(`{"type":"response.create"}`, session, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}), account)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		result <- err
	}()
	select {
	case err := <-frameWritten:
		if err != nil {
			t.Fatalf("precommit frame was not consumed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("precommit synthetic upstream stalled")
	}
	// The net.Pipe writer has returned: the frame bytes were consumed.
	// Allow the event parser to finish without relying on a long timeout.
	until := time.Now().Add(150 * time.Millisecond)
	for {
		watchdog.mu.Lock()
		timeout := watchdog.timeout
		watchdog.mu.Unlock()
		if timeout != websocketPrecommitProgressTimeout {
			t.Fatalf("hidden prelude extended precommit read budget to %s; want %s until actual commitment", timeout, websocketPrecommitProgressTimeout)
		}
		if time.Now().After(until) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation while precommit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hidden prelude left blocked read after cancellation")
	}
}

// Even repeated hidden prelude frames are not completion. The original
// router-owned request deadline wins over per-frame traffic, and a later
// ordinary request can use the same profile without stale retry penalties.
func TestProdex04382RepeatedHiddenPreludeRespectsOriginalDeadlineAndFreshRequest(t *testing.T) {
	var attempts atomic.Int32
	var preludes atomic.Int32
	upstreamClosed := make(chan struct{}, 1)
	server := newWebSocketMessageParityServer(t, nil, func(conn net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			return
		}
		if attempts.Add(1) == 1 {
			for i := 0; i < 120; i++ {
				payload := []byte(fmt.Sprintf(`{"type":"response.created","response":{"id":"pending","metadata":{"part":%d}}}`, i))
				if writeWebSocketParityEvent(conn, payload) != nil {
					break
				}
				preludes.Add(1)
				time.Sleep(3 * time.Millisecond)
			}
			select {
			case upstreamClosed <- struct{}{}:
			default:
			}
			return
		}
		_ = writeWebSocketParityEvent(conn, []byte(`{"type":"response.completed","response":{"id":"resp-fresh"}}`))
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	account := proxymodel.Account{ID: "synthetic-a", Home: "synthetic-home", Enabled: true}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: transport, PrecommitWatchdogBudget: 110 * time.Millisecond,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	req := websocketParityRequest(`{"type":"response.create"}`, 7192, proxymodel.WebSocketPolicy{})
	req.QuotaSelection = quotamodel.Selection{RouteKind: quotamodel.RouteKindWebSocket}
	started := time.Now()
	exchange, err := router.Forward(context.Background(), req)
	if exchange != nil {
		_ = exchange.Close()
		t.Fatal("hidden prelude incorrectly committed a response")
	}
	var status *proxymodel.Error
	if !errors.As(err, &status) || status.StatusCode != http.StatusGatewayTimeout ||
		!strings.Contains(status.Message, "request timed out") {
		t.Fatalf("repeated hidden prelude did not return local request_timeout 504: %v", err)
	}
	if time.Since(started) > time.Second || preludes.Load() < 2 {
		t.Fatalf("precommit progress renewed request deadline or fixture lacked progress: elapsed=%s preludes=%d", time.Since(started), preludes.Load())
	}
	// A subsequent new turn must not inherit the timed-out attempt's failed
	// profile exclusion, old deadline or socket. It can use the same profile.
	follow := websocketParityRequest(`{"type":"response.create"}`, 7193, proxymodel.WebSocketPolicy{})
	follow.QuotaSelection = req.QuotaSelection
	next, err := router.Forward(context.Background(), follow)
	if err != nil {
		t.Fatalf("fresh request after typed 504 was blocked: %v", err)
	}
	data, readErr := io.ReadAll(next.Result.Response.Body)
	closeErr := next.Close()
	if readErr != nil || closeErr != nil || !strings.Contains(string(data), "resp-fresh") || attempts.Load() != 2 {
		t.Fatalf("fresh request after precommit timeout: body=%q read=%v close=%v attempts=%d", data, readErr, closeErr, attempts.Load())
	}
}
