package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestWebSocketIngressMatchesProdexUpgradeDetection(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://example.test/backend-api/codex/responses", nil)
	request.Header.Add("Upgrade", "h2c")
	request.Header.Add("Upgrade", "WebSocket")
	request.Header.Set("Sec-WebSocket-Key", "not-base64-and-still-valid-for-local-accept")
	if !isWebSocketUpgradeRequest(request) {
		t.Fatal("exact websocket Upgrade header was not detected")
	}
	if status, message := websocketRequestError(request); status != 0 {
		t.Fatalf("Prodex-compatible local handshake rejected: %d %q", status, message)
	}

	notWebSocket := httptest.NewRequest(http.MethodGet, "http://example.test/backend-api/codex/responses", nil)
	notWebSocket.Header.Set("Upgrade", "h2c, websocket")
	if isWebSocketUpgradeRequest(notWebSocket) {
		t.Fatal("comma-combined non-exact Upgrade value was treated as websocket")
	}
}

func TestSupportedWebSocketPathsMatchProdexRuntimePlanner(t *testing.T) {
	for _, path := range []string{
		"/backend-api/codex/responses",
		"/backend-api/prodex/responses",
		"/backend-api/prodex/v0.2.99/responses",
		"/backend-api/prodex/victim/realtime",
		"/backend-api/codex/nested/realtime",
		"/backend-api/codex/live",
		"/backend-api/codex/live/call-123",
		"/backend-api/prodex/v0.2.99/live/call-123",
		"/v1/realtime",
		"/v1/live",
		"/other/live/call-123",
	} {
		if !supportedWebSocketPath(path) {
			t.Errorf("Prodex-supported websocket path rejected: %s", path)
		}
	}
	for _, path := range []string{
		"/responses",
		"/v1/responses",
		"/backend-api/prodex/v0.2.99/realtime/calls",
		"/backend-api/codex/live/call-123/extra",
		"/backend-api/codex/responses/compact",
		"/unrelated",
	} {
		if supportedWebSocketPath(path) {
			t.Errorf("Prodex-unsupported websocket path accepted: %s", path)
		}
	}
}

func TestWebSocketLocalHandshakeUsesClientKeyAndHidesUpstreamHeaders(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "managed-secret", "B", "token-b")
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(buffered,
			"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\nSec-WebSocket-Protocol: upstream.protocol\r\nSec-WebSocket-Extensions: permessage-deflate\r\nX-Upstream-Only: hidden\r\n\r\n",
			websocketAccept(request.Header.Get("Sec-WebSocket-Key")),
		)
		_ = buffered.Flush()
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	connection, reader := dialWebSocket(t, proxy.URL, "/backend-api/codex/responses", "")
	defer connection.Close()
	status, headers, _ := readWebSocketHandshake(t, reader)
	if status != http.StatusSwitchingProtocols ||
		headers.Get("Upgrade") != "websocket" || !strings.EqualFold(headers.Get("Connection"), "Upgrade") ||
		headers.Get("Sec-WebSocket-Accept") != websocketAccept(websocketTestKey) ||
		headers.Get("Sec-WebSocket-Protocol") != "" || headers.Get("Sec-WebSocket-Extensions") != "" ||
		headers.Get("X-Upstream-Only") != "" {
		t.Fatalf("local handshake = status:%d headers:%#v", status, headers)
	}
}

type websocketPipeGateway struct {
	peerReady    chan net.Conn
	requestReady chan proxymodel.Request
}

func (gateway *websocketPipeGateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, nil
}

func (gateway *websocketPipeGateway) ExecuteWebSocketMessage(_ context.Context, request proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	body, peer := net.Pipe()
	gateway.requestReady <- request
	gateway.peerReady <- peer
	return &proxymodel.Response{
		StatusCode:              http.StatusSwitchingProtocols,
		Header:                  make(http.Header),
		Body:                    body,
		WebSocketFrames:         true,
		WebSocketRealtimeDuplex: true,
		FirstEventCommitted:     true,
	}, nil
}

func (gateway *websocketPipeGateway) CloseWebSocketSession(uint64) {}

func TestProxyRealtimeWebSocketReturnsLocalHandshakeBeforeSelectingUpstream(t *testing.T) {
	gateway := &websocketPipeGateway{
		peerReady: make(chan net.Conn, 1), requestReady: make(chan proxymodel.Request, 1),
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "A", Home: t.TempDir(), Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	connection, reader := dialWebSocket(t, server.URL, "/backend-api/codex/realtime", "")
	defer connection.Close()
	status, headers, _ := readWebSocketHandshake(t, reader)
	if status != http.StatusSwitchingProtocols || headers.Get("Sec-WebSocket-Accept") != websocketAccept(websocketTestKey) {
		t.Fatalf("local handshake = %d %#v", status, headers)
	}
	select {
	case request := <-gateway.requestReady:
		t.Fatalf("realtime selected upstream before first text: %#v", request)
	default:
	}

	first := `{"type":"session.update"}`
	if _, err := connection.Write(maskedWebSocketFrame(1, true, first)); err != nil {
		t.Fatal(err)
	}
	var routed proxymodel.Request
	select {
	case routed = <-gateway.requestReady:
	case <-time.After(time.Second):
		t.Fatal("first realtime text did not select upstream")
	}
	if string(routed.Body) != first || !routed.WebSocketMessage || !routed.WebSocketPolicy.RealtimeDuplex {
		t.Fatalf("routed realtime request = %#v", routed)
	}
	var peer net.Conn
	select {
	case peer = <-gateway.peerReady:
		defer peer.Close()
	case <-time.After(time.Second):
		t.Fatal("first realtime text did not create websocket duplex body")
	}

	second := `{"type":"input_audio.append","audio":"x"}`
	if _, err := connection.Write(maskedWebSocketFrame(1, true, second)); err != nil {
		t.Fatal(err)
	}
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	frame, err := websocketframe.ReadHeader(peer)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := frame.ReadPayload(peer, websocketDefaultMaxFrameBytes)
	if err != nil {
		t.Fatal(err)
	}
	frame.Unmask(payload)
	if frame.Opcode != 1 || string(payload) != second {
		t.Fatalf("client-to-upstream frame = opcode %d payload %q", frame.Opcode, payload)
	}

	if err := websocketframe.WriteFrame(peer, 1, []byte(`{"type":"output_audio.delta","delta":"r"}`), false); err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	opcode, payload, err := readWebSocketTestFrame(reader)
	if err != nil || opcode != 1 || !strings.Contains(string(payload), `"delta":"r"`) {
		t.Fatalf("upstream-to-client frame = opcode %d payload %q err=%v", opcode, payload, err)
	}
}
