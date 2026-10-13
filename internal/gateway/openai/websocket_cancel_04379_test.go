package openai

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

// A logical WebSocket message must not wait for the eight-second idle timer
// once its owner request has been cancelled before any model output.
func TestProdex04379WebSocketPrecommitCancelledReadReleasesUpstream(t *testing.T) {
	transport, err := NewTransport("http://127.0.0.1", nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	account := proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}
	client, server := net.Pipe()
	defer server.Close()
	transport.recycleWebSocketMessageSession(798, account, client, "")

	readRequest := make(chan error, 1)
	upstreamClosed := make(chan error, 1)
	go func() {
		_, err := readWebSocketParityRequest(bufio.NewReader(server))
		readRequest <- err
		if err != nil {
			return
		}
		_, err = server.Read(make([]byte, 1))
		upstreamClosed <- err
	}()
	request := websocketParityRequest(`{"type":"response.create"}`, 798, proxymodel.WebSocketPolicy{
		PromoteCommittedProfile: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		response, err := transport.ExecuteWebSocketMessage(ctx, request, account)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		done <- err
	}()
	select {
	case err := <-readRequest:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		_ = server.Close()
		t.Fatal("websocket request did not reach the synthetic upstream")
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled precommit request returned %v, want context cancellation", err)
		}
	case <-time.After(350 * time.Millisecond):
		_ = server.Close()
		<-done
		t.Fatal("precommit cancellation waited for the idle timeout")
	}
	select {
	case err := <-upstreamClosed:
		if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("cancelled upstream read = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled upstream connection was retained")
	}
}

// A precommit deadline must be disarmed once real output has been committed.
// The native client may continue reading the same upstream WebSocket afterwards.
func TestProdex04379CommittedWebSocketSurvivesRequestContextCancellation(t *testing.T) {
	releaseTerminal := make(chan struct{})
	output := []byte(`{"type":"response.output_text.delta","delta":"visible"}`)
	completed := []byte(`{"type":"response.completed","response":{"id":"resp-visible"}}`)
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read native request: %v", err)
			return
		}
		if err := writeWebSocketParityEvent(connection, output); err != nil {
			t.Errorf("write output: %v", err)
			return
		}
		<-releaseTerminal
		_ = writeWebSocketParityEvent(connection, completed)
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	response, err := transport.ExecuteWebSocketMessage(
		ctx,
		websocketParityRequest(`{"type":"response.create"}`, 799,
			proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
		proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
	)
	if err != nil {
		cancel()
		close(releaseTerminal)
		t.Fatal(err)
	}
	defer response.Body.Close()
	if !response.FirstEventCommitted || response.PrecommitFailure != nil {
		cancel()
		close(releaseTerminal)
		t.Fatalf("output was not committed: %#v", response)
	}
	cancel()
	close(releaseTerminal)
	actual, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("committed stream was interrupted by request cancellation: %v", err)
	}
	if expected := websocketParityFrames(t, output, completed); !bytes.Equal(actual, expected) {
		t.Fatalf("committed stream payload length = %d, want %d", len(actual), len(expected))
	}
}

// The five-minute request-scoped watchdog (shortened only by the fixture)
// must win over the gateway's eight-second per-read timeout. It is a typed
// local 504 and does not misclassify the profile as quota-exhausted.
func TestProdex04379RealWebSocketMessagePrecommitWatchdogReturns504(t *testing.T) {
	upstreamClosed := make(chan error, 1)
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read upstream request: %v", err)
			return
		}
		_, err := connection.Read(make([]byte, 1))
		upstreamClosed <- err
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	account := proxymodel.Account{ID: "websocket-a", Home: "synthetic-home", Enabled: true}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway:                 transport,
		PrecommitWatchdogBudget: 70 * time.Millisecond,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{account}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	request := websocketParityRequest(`{"type":"response.create"}`, 800, proxymodel.WebSocketPolicy{})
	request.QuotaSelection = quotamodel.Selection{RouteKind: quotamodel.RouteKindWebSocket}
	started := time.Now()
	exchange, err := router.Forward(context.Background(), request)
	if exchange != nil {
		_ = exchange.Close()
		t.Fatal("timed-out websocket message unexpectedly committed")
	}
	var presented *proxymodel.Error
	if !errors.As(err, &presented) || presented.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("precommit WebSocket timeout=%v, want HTTP 504", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("precommit watchdog exceeded its bounded request budget: %s", elapsed)
	}
	select {
	case err := <-upstreamClosed:
		if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("upstream closure after 504 = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog returned 504 without releasing the upstream socket")
	}
}
