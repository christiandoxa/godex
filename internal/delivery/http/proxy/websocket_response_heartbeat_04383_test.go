package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

// The real local Responses upgrade must emit only transport control frames
// while the upstream is quiet, including before any precommit output. The
// local Ping MUST NOT become inference output, count as a second dispatch or
// create a fresh context deadline. A followup remains on the same socket.
type quietResponsesGateway04383 struct {
	gate  chan struct{}
	mu    sync.Mutex
	calls int
}

func (g *quietResponsesGateway04383) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("unexpected regular HTTP request")
}
func (g *quietResponsesGateway04383) ExecuteWebSocketMessage(ctx context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	g.mu.Lock()
	g.calls++
	index := g.calls
	g.mu.Unlock()
	if account.ID != "ready" || !request.WebSocketMessage {
		return nil, fmt.Errorf("wrong WebSocket owner or request")
	}
	if index == 1 {
		select {
		case <-g.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	var frames bytes.Buffer
	if err := websocketframe.WriteFrame(&frames, 1, []byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp-%d"}}`, index)), false); err != nil {
		return nil, err
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(frames.Bytes())),
		WebSocketFrames: true, FirstEventCommitted: true,
		WebSocketResponseID: fmt.Sprintf("resp-%d", index),
	}, nil
}

func TestProdex04383QuietResponsesTransportPingsBeforeCommitWithoutReplay(t *testing.T) {
	gateway := &quietResponsesGateway04383{gate: make(chan struct{})}
	defer func() {
		select {
		case <-gateway.gate:
		default:
			close(gateway.gate)
		}
	}()
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "ready", Home: "synthetic-home", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	proxy, err := NewProxy(Config{Router: router, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()
	client, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/codex/responses")
	defer client.Close()
	if status, _ := readResponsesPublicHandshake(t, reader); status != http.StatusSwitchingProtocols {
		t.Fatalf("status=%d", status)
	}
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	if _, err := client.Write(protocolClientFrame(1, true, []byte(`{"type":"response.create","input":[]}`), true)); err != nil {
		t.Fatal(err)
	}
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		t.Fatalf("local client did not receive precommit Ping: %v", err)
	}
	payload, err := frame.ReadPayload(reader, 125)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Opcode != 9 || len(payload) != 0 || elapsedOutside(time.Since(start), 850*time.Millisecond, 1800*time.Millisecond) {
		t.Fatalf("quiet precommit liveness: opcode=%d payload=%q elapsed=%s", frame.Opcode, payload, time.Since(start))
	}
	gateway.mu.Lock()
	calls := gateway.calls
	gateway.mu.Unlock()
	if calls != 1 {
		t.Fatalf("heartbeat created duplicate inference: %d", calls)
	}
	if _, err := client.Write(protocolClientFrame(10, true, nil, true)); err != nil {
		t.Fatal(err)
	}
	close(gateway.gate)
	if event := readResponsesPublicTextFrame(t, reader); event != `{"type":"response.completed","response":{"id":"resp-1"}}` {
		t.Fatalf("precommit heartbeat corrupted/lost terminal event: %q", event)
	}
	if _, err := client.Write(protocolClientFrame(1, true, []byte(`{"type":"response.create","input":[]}`), true)); err != nil {
		t.Fatal(err)
	}
	if event := readResponsesPublicTextFrame(t, reader); event != `{"type":"response.completed","response":{"id":"resp-2"}}` {
		t.Fatalf("Pong prevented next turn or rewrote response: %q", event)
	}
	gateway.mu.Lock()
	calls = gateway.calls
	gateway.mu.Unlock()
	if calls != 2 {
		t.Fatalf("followup count=%d want exactly 2", calls)
	}
}

func elapsedOutside(value, minimum, maximum time.Duration) bool {
	return value < minimum || value > maximum
}
