package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type realtimePublicGateway struct {
	mu       sync.Mutex
	accounts []string
	calls    chan proxymodel.Request
	peers    chan net.Conn
}

func (gateway *realtimePublicGateway) Execute(
	context.Context,
	proxymodel.Request,
	proxymodel.Account,
) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("standard execute must not handle realtime websocket")
}

func (gateway *realtimePublicGateway) ExecuteWebSocketMessage(
	_ context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	client, peer := net.Pipe()
	gateway.mu.Lock()
	gateway.accounts = append(gateway.accounts, account.ID)
	gateway.mu.Unlock()
	gateway.calls <- request
	gateway.peers <- peer
	return &proxymodel.Response{
		StatusCode:              http.StatusSwitchingProtocols,
		Header:                  make(http.Header),
		Body:                    client,
		WebSocketFrames:         true,
		WebSocketRealtimeDuplex: true,
		FirstEventCommitted:     true,
	}, nil
}

func (gateway *realtimePublicGateway) CloseWebSocketSession(uint64) {}

func TestPublicRealtimeWebSocketCommitsOnFirstTextThenPumpsDuplex(t *testing.T) {
	gateway := &realtimePublicGateway{
		calls: make(chan proxymodel.Request, 1),
		peers: make(chan net.Conn, 1),
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/prodex/live")
	defer connection.Close()
	status, _ := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("realtime local handshake = %d", status)
	}
	select {
	case request := <-gateway.calls:
		t.Fatalf("realtime selected upstream before first text: %#v", request)
	default:
	}

	first := []byte(`{"type":"session.update"}`)
	if _, err := connection.Write(protocolClientFrame(1, true, first, true)); err != nil {
		t.Fatal(err)
	}
	var routed proxymodel.Request
	select {
	case routed = <-gateway.calls:
	case <-time.After(time.Second):
		t.Fatal("first realtime text did not reach router")
	}
	if string(routed.Body) != string(first) ||
		!routed.WebSocketMessage ||
		!routed.WebSocketPolicy.RealtimeDuplex ||
		routed.WebSocketSessionID == 0 {
		t.Fatalf("routed realtime request = %#v", routed)
	}
	peer := <-gateway.peers
	defer peer.Close()
	_ = peer.SetDeadline(time.Now().Add(2 * time.Second))
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))

	second := []byte(`{"type":"input_audio.append","audio":"chunk-one"}`)
	third := []byte(`{"type":"input_audio.append","audio":"chunk-two"}`)
	if _, err := connection.Write(protocolClientFrame(1, true, second, true)); err != nil {
		t.Fatal(err)
	}
	assertRealtimePeerFrame(t, peer, 1, second)

	if _, err := connection.Write(protocolClientFrame(9, true, []byte("local-ping"), true)); err != nil {
		t.Fatal(err)
	}
	opcode, payload := readRealtimePublicFrame(t, reader)
	if opcode != 10 || string(payload) != "local-ping" {
		t.Fatalf("local pong = opcode %d payload %q", opcode, payload)
	}

	if _, err := connection.Write(protocolClientFrame(2, true, []byte{1, 2, 3}, true)); err != nil {
		t.Fatal(err)
	}
	assertRealtimePeerFrame(t, peer, 2, []byte{1, 2, 3})

	if _, err := connection.Write(protocolClientFrame(1, true, third, true)); err != nil {
		t.Fatal(err)
	}
	assertRealtimePeerFrame(t, peer, 1, third)

	if err := websocketframe.WriteFrame(peer, 9, []byte("upstream-ping"), false); err != nil {
		t.Fatal(err)
	}
	assertRealtimePeerFrame(t, peer, 10, []byte("upstream-ping"))

	output := []byte(`{"type":"output_audio.delta","delta":"fixture-audio"}`)
	if err := websocketframe.WriteFrame(peer, 1, output, false); err != nil {
		t.Fatal(err)
	}
	if err := websocketframe.WriteFrame(peer, 2, []byte{9, 8, 7}, false); err != nil {
		t.Fatal(err)
	}
	opcode, payload = readRealtimePublicFrame(t, reader)
	if opcode != 1 || !bytes.Equal(payload, output) {
		t.Fatalf("realtime text output = opcode %d payload %q", opcode, payload)
	}
	opcode, payload = readRealtimePublicFrame(t, reader)
	if opcode != 2 || !bytes.Equal(payload, []byte{9, 8, 7}) {
		t.Fatalf("realtime binary output = opcode %d payload %v", opcode, payload)
	}

	closePayload := []byte{0x03, 0xe8}
	if _, err := connection.Write(protocolClientFrame(8, true, closePayload, true)); err != nil {
		t.Fatal(err)
	}
	assertRealtimePeerFrame(t, peer, 8, closePayload)
	opcode, payload = readRealtimePublicFrame(t, reader)
	if opcode != 8 || !bytes.Equal(payload, closePayload) {
		t.Fatalf("local close echo = opcode %d payload %v", opcode, payload)
	}

	gateway.mu.Lock()
	accounts := append([]string(nil), gateway.accounts...)
	gateway.mu.Unlock()
	if len(accounts) != 1 || accounts[0] != "account-a" {
		t.Fatalf("realtime owner attempts = %v", accounts)
	}
}

func readRealtimePublicFrame(t *testing.T, reader io.Reader) (byte, []byte) {
	t.Helper()
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := frame.ReadPayload(reader, websocketDefaultMaxFrameBytes)
	if err != nil {
		t.Fatal(err)
	}
	frame.Unmask(payload)
	return frame.Opcode, payload
}

func assertRealtimePeerFrame(t *testing.T, peer io.Reader, wantOpcode byte, wantPayload []byte) {
	t.Helper()
	frame, err := websocketframe.ReadHeader(peer)
	if err != nil {
		t.Fatal(err)
	}
	if !frame.Masked() {
		t.Fatalf("upstream client frame opcode %d was not masked", frame.Opcode)
	}
	payload, err := frame.ReadPayload(peer, websocketDefaultMaxFrameBytes)
	if err != nil {
		t.Fatal(err)
	}
	frame.Unmask(payload)
	if frame.Opcode != wantOpcode || !bytes.Equal(payload, wantPayload) {
		t.Fatalf("upstream frame = opcode %d payload %v, want %d %v", frame.Opcode, payload, wantOpcode, wantPayload)
	}
}
