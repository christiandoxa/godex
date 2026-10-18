package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type shutdownReleaseConn04382 struct {
	net.Conn
	onClose func()
}

func (connection *shutdownReleaseConn04382) Close() error {
	err := connection.Conn.Close()
	connection.onClose()
	return err
}

func TestProdex04382ProxyShutdownStopsAdmissionBeforeReleasingLivePermit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	waitEntered := make(chan struct{}, 1)
	gateway := &realtimePublicGateway{calls: make(chan proxymodel.Request, 2), peers: make(chan net.Conn, 2)}
	defer func() {
		for len(gateway.peers) > 0 {
			_ = (<-gateway.peers).Close()
		}
	}()
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "synthetic-ready", Home: "/synthetic", Enabled: true}}, nil
		},
		ProfileInflightWait: func(ctx context.Context, changed <-chan struct{}, _ time.Duration) error {
			select {
			case waitEntered <- struct{}{}:
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
	proxy, err := NewProxy(Config{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	request := proxymodel.Request{
		Method: http.MethodGet, Path: "/backend-api/godex/live",
		Header: http.Header{"Upgrade": {"websocket"}},
		Body:   []byte(`{"type":"session.update"}`), WebSocketMessage: true,
		WebSocketPolicy: proxymodel.WebSocketPolicy{RealtimeDuplex: true},
	}
	live, err := router.Forward(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	follower := request
	follower.Path = "/backend-api/godex/responses"
	follower.Body = []byte(`{"type":"response.create"}`)
	follower.WebSocketPolicy = proxymodel.WebSocketPolicy{}
	completed := make(chan error, 1)
	go func() {
		exchange, err := router.Forward(ctx, follower)
		if exchange != nil {
			_ = exchange.Close()
		}
		completed <- err
	}()
	select {
	case <-waitEntered:
	case <-ctx.Done():
		t.Fatal("follower did not wait for the live permit")
	}
	client, peer := net.Pipe()
	defer peer.Close()
	var followerErr error
	proxy.responsesWebSocketTunnels[&responsesWebSocketTunnel{client: &shutdownReleaseConn04382{
		Conn: client,
		onClose: func() {
			_ = live.Close()
			select {
			case followerErr = <-completed:
			case <-ctx.Done():
				followerErr = ctx.Err()
			}
		},
	}}] = struct{}{}
	if err := proxy.Close(ctx); err != nil {
		t.Fatal(err)
	}
	gateway.mu.Lock()
	calls := len(gateway.accounts)
	gateway.mu.Unlock()
	if calls != 1 {
		t.Fatalf("shutdown released live capacity into a new model dispatch: %d calls", calls)
	}
	if !errors.Is(followerErr, context.Canceled) {
		t.Fatalf("queued request did not stop with shutdown: %v", followerErr)
	}
}
