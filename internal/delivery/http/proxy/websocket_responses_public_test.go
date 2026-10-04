package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

const responsesWebSocketTestKey = "dGhlIHNhbXBsZSBub25jZQ=="

type responsesPublicGateway struct {
	mu       sync.Mutex
	accounts []string
	bodies   []string
	sessions []uint64
	closed   chan uint64
}

func (gateway *responsesPublicGateway) Execute(
	context.Context, proxymodel.Request, proxymodel.Account,
) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("standard execute must not handle public responses websocket")
}

func (gateway *responsesPublicGateway) ExecuteWebSocketMessage(
	_ context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	index := len(gateway.bodies) + 1
	gateway.accounts = append(gateway.accounts, account.ID)
	gateway.bodies = append(gateway.bodies, string(request.Body))
	gateway.sessions = append(gateway.sessions, request.WebSocketSessionID)
	gateway.mu.Unlock()

	responseID := fmt.Sprintf("resp-%d", index)
	payload := []byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"%s"}}`, responseID))
	var frames bytes.Buffer
	if err := websocketframe.WriteFrame(&frames, 1, payload, false); err != nil {
		return nil, err
	}
	return &proxymodel.Response{
		StatusCode:          http.StatusOK,
		Header:              make(http.Header),
		Body:                io.NopCloser(bytes.NewReader(frames.Bytes())),
		WebSocketFrames:     true,
		FirstEventCommitted: true,
		WebSocketResponseID: responseID,
	}, nil
}

func (gateway *responsesPublicGateway) CloseWebSocketSession(sessionID uint64) {
	select {
	case gateway.closed <- sessionID:
	default:
	}
}

func newResponsesPublicProxy(t *testing.T, gateway *responsesPublicGateway) *Proxy {
	t.Helper()
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "account-a", Home: "/a", Enabled: true},
				{ID: "account-b", Home: "/b", Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	return proxy
}

func TestPublicResponsesWebSocketRoutesMessagesThroughCommittedOwners(t *testing.T) {
	gateway := &responsesPublicGateway{closed: make(chan uint64, 1)}
	proxy := newResponsesPublicProxy(t, gateway)
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/codex/responses")
	defer connection.Close()
	status, headers := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols ||
		headers.Get("Sec-WebSocket-Accept") != responsesWebSocketAccept(responsesWebSocketTestKey) ||
		headers.Get("Sec-WebSocket-Protocol") != "" || headers.Get("Sec-WebSocket-Extensions") != "" {
		t.Fatalf("local handshake = %d %#v", status, headers)
	}

	first := `{"type":"response.create","response":{}}`
	second := `{"type":"response.create","previous_response_id":"resp-1","response":{"previous_response_id":"resp-1"}}`
	if _, err := connection.Write(protocolClientFrame(1, true, []byte(first), true)); err != nil {
		t.Fatal(err)
	}
	if got := readResponsesPublicTextFrame(t, reader); !strings.Contains(got, `"id":"resp-1"`) {
		t.Fatalf("first response = %q", got)
	}
	if _, err := connection.Write(protocolClientFrame(1, true, []byte(second), true)); err != nil {
		t.Fatal(err)
	}
	if got := readResponsesPublicTextFrame(t, reader); !strings.Contains(got, `"id":"resp-2"`) {
		t.Fatalf("second response = %q", got)
	}

	gateway.mu.Lock()
	accounts := append([]string(nil), gateway.accounts...)
	bodies := append([]string(nil), gateway.bodies...)
	sessions := append([]uint64(nil), gateway.sessions...)
	gateway.mu.Unlock()
	if len(accounts) != 2 || accounts[0] != accounts[1] ||
		!strings.Contains(bodies[1], `"previous_response_id":"resp-1"`) ||
		len(sessions) != 2 || sessions[0] == 0 || sessions[0] != sessions[1] {
		t.Fatalf("accounts/bodies/sessions = %#v/%#v/%#v", accounts, bodies, sessions)
	}

	closePayload := []byte{0x03, 0xe8}
	if _, err := connection.Write(protocolClientFrame(8, true, closePayload, true)); err != nil {
		t.Fatal(err)
	}
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := frame.ReadPayload(reader, 125)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Opcode != 8 || !bytes.Equal(payload, closePayload) {
		t.Fatalf("close echo = opcode %d payload %v", frame.Opcode, payload)
	}
	select {
	case closed := <-gateway.closed:
		if closed != sessions[0] {
			t.Fatalf("closed session = %d, want %d", closed, sessions[0])
		}
	case <-time.After(time.Second):
		t.Fatal("websocket upstream session was not released")
	}
}

func TestPublicResponsesWebSocketRejectsBinaryAndContinues(t *testing.T) {
	gateway := &responsesPublicGateway{closed: make(chan uint64, 1)}
	proxy := newResponsesPublicProxy(t, gateway)
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/codex/responses")
	defer connection.Close()
	status, _ := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d", status)
	}
	frames := append(
		protocolClientFrame(2, true, []byte("binary"), true),
		protocolClientFrame(1, true, []byte(`{"type":"response.create","response":{}}`), true)...,
	)
	if _, err := connection.Write(frames); err != nil {
		t.Fatal(err)
	}
	if got := readResponsesPublicTextFrame(t, reader); !strings.Contains(got, `"code":"invalid_request_error"`) || !strings.Contains(got, `"status":400`) {
		t.Fatalf("binary rejection = %q", got)
	}
	if got := readResponsesPublicTextFrame(t, reader); !strings.Contains(got, `"id":"resp-1"`) {
		t.Fatalf("continued response = %q", got)
	}
	gateway.mu.Lock()
	calls := len(gateway.bodies)
	gateway.mu.Unlock()
	if calls != 1 {
		t.Fatalf("binary frame reached router; routed calls=%d", calls)
	}
}

func TestPublicResponsesWebSocketKeepsOtherUpgradeSurfacesFailClosed(t *testing.T) {
	gateway := &responsesPublicGateway{closed: make(chan uint64, 1)}
	proxy := newResponsesPublicProxy(t, gateway)
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/v1/realtime")
	defer connection.Close()
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("realtime websocket status = %d, want 426", response.StatusCode)
	}
	gateway.mu.Lock()
	calls := len(gateway.bodies)
	gateway.mu.Unlock()
	if calls != 0 {
		t.Fatalf("unsupported websocket route reached message gateway: %d calls", calls)
	}
}

func TestProxyCloseClosesPublicResponsesWebSocket(t *testing.T) {
	gateway := &responsesPublicGateway{closed: make(chan uint64, 1)}
	proxy := newResponsesPublicProxy(t, gateway)
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	connection, reader := dialResponsesPublicWebSocket(t, proxy.Endpoint(), "/backend-api/codex/responses")
	defer connection.Close()
	status, _ := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d", status)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := proxy.Close(ctx); err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("proxy close left public websocket open")
	}
	select {
	case <-gateway.closed:
	case <-time.After(time.Second):
		t.Fatal("proxy close did not release websocket message session")
	}
}

func dialResponsesPublicWebSocket(t *testing.T, endpoint, path string) (net.Conn, *bufio.Reader) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.Dial("tcp", request.URL.Host)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(connection)
	_, err = fmt.Fprintf(
		connection,
		"GET %s HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Protocol: codex.realtime\r\nSec-WebSocket-Extensions: permessage-deflate\r\n\r\n",
		path, request.URL.Host, responsesWebSocketTestKey,
	)
	if err != nil {
		_ = connection.Close()
		t.Fatal(err)
	}
	return connection, reader
}

func readResponsesPublicHandshake(t *testing.T, reader *bufio.Reader) (int, http.Header) {
	t.Helper()
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var status int
	if _, err := fmt.Sscanf(statusLine, "HTTP/1.1 %d", &status); err != nil {
		t.Fatalf("status line %q: %v", statusLine, err)
	}
	headers := make(http.Header)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			return status, headers
		}
		name, value, ok := strings.Cut(strings.TrimRight(line, "\r\n"), ":")
		if !ok {
			t.Fatalf("invalid header %q", line)
		}
		headers.Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}
}

func readResponsesPublicTextFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := frame.ReadPayload(reader, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Opcode != 1 {
		t.Fatalf("opcode = %d, want text", frame.Opcode)
	}
	return string(payload)
}

func responsesWebSocketAccept(key string) string {
	digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(digest[:])
}
