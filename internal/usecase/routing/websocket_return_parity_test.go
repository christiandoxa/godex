package routing

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketReturnGateway struct {
	peer net.Conn
}

func (gateway *websocketReturnGateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, nil
}

func (gateway *websocketReturnGateway) ExecuteWebSocket(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	body, peer := net.Pipe()
	gateway.peer = peer
	return &proxymodel.Response{
		StatusCode: http.StatusSwitchingProtocols,
		Header: http.Header{
			"Upgrade":              []string{"websocket"},
			"Connection":           []string{"Upgrade"},
			"Sec-WebSocket-Accept": []string{"synthetic"},
		},
		Body: body,
	}, nil
}

func TestRouterReturnsWebSocketUpgradeWithoutReadingDuplexBody(t *testing.T) {
	gateway := &websocketReturnGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "A", Home: t.TempDir(), Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	headers := make(http.Header)
	headers.Set("Upgrade", "websocket")
	result := make(chan struct {
		exchange *Exchange
		err      error
	}, 1)
	go func() {
		exchange, forwardErr := router.Forward(context.Background(), proxymodel.Request{
			Method: http.MethodGet, Path: "/backend-api/codex/responses", Header: headers,
		})
		result <- struct {
			exchange *Exchange
			err      error
		}{exchange: exchange, err: forwardErr}
	}()
	select {
	case outcome := <-result:
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.exchange == nil || outcome.exchange.Result.Response.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("exchange = %#v", outcome.exchange)
		}
		duplex, ok := outcome.exchange.Result.Response.Body.(io.ReadWriteCloser)
		if !ok {
			t.Fatalf("response body type = %T", outcome.exchange.Result.Response.Body)
		}
		_ = duplex.Close()
		_ = gateway.peer.Close()
		_ = outcome.exchange.Close()
	case <-time.After(time.Second):
		t.Fatal("router blocked on websocket duplex body")
	}
}

type websocketRetryGateway struct {
	firstStatus int
	firstBody   string
	firstError  error
	calls       []string
	peers       []net.Conn
}

func (gateway *websocketRetryGateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, nil
}

func (gateway *websocketRetryGateway) ExecuteWebSocket(_ context.Context, _ proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.calls = append(gateway.calls, account.ID)
	if account.ID == "A" {
		if gateway.firstError != nil {
			return nil, gateway.firstError
		}
		return &proxymodel.Response{
			StatusCode: gateway.firstStatus,
			Header:     http.Header{"Retry-After": []string{"0"}},
			Body:       io.NopCloser(strings.NewReader(gateway.firstBody)),
		}, nil
	}
	body, peer := net.Pipe()
	gateway.peers = append(gateway.peers, peer)
	return &proxymodel.Response{
		StatusCode: http.StatusSwitchingProtocols,
		Header: http.Header{
			"Upgrade":    []string{"websocket"},
			"Connection": []string{"Upgrade"},
		},
		Body: body,
	}, nil
}

func TestRouterRetriesWebSocketHandshakeFailuresBeforeCommit(t *testing.T) {
	for _, fixture := range []struct {
		name         string
		status       int
		body         string
		transportErr error
	}{
		{name: "rate limit", status: http.StatusTooManyRequests, body: `{"error":{"code":"rate_limit_exceeded"}}`},
		{name: "overload", status: http.StatusServiceUnavailable},
		{name: "transport failure", transportErr: errors.New("synthetic connect failure")},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &websocketRetryGateway{
				firstStatus: fixture.status,
				firstBody:   fixture.body,
				firstError:  fixture.transportErr,
			}
			router, err := NewRouter(Config{
				Gateway: gateway,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{
						{ID: "A", Home: "/synthetic/a", Enabled: true},
						{ID: "B", Home: "/synthetic/b", Enabled: true},
					}, nil
				},
				PreferredAccount: "A",
			})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			defer func() {
				for _, peer := range gateway.peers {
					_ = peer.Close()
				}
			}()

			headers := make(http.Header)
			headers.Set("Upgrade", "websocket")
			headers.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			exchange, err := router.Forward(context.Background(), proxymodel.Request{
				Method: http.MethodGet, Path: "/backend-api/codex/responses", Header: headers,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()

			if len(gateway.calls) != 2 || gateway.calls[0] != "A" || gateway.calls[1] != "B" {
				t.Fatalf("websocket attempts = %v, want [A B]", gateway.calls)
			}
			if exchange.Result.AccountID != "B" || exchange.Result.Response.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("final websocket owner/status = %q/%d, want B/101", exchange.Result.AccountID, exchange.Result.Response.StatusCode)
			}
		})
	}
}
