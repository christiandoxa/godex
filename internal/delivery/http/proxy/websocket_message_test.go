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

func TestResponsesWebSocketRoutesEachMessageWithResponseAffinity(t *testing.T) {
	gateway := &websocketMessageTestGateway{closedSessions: make(chan uint64, 1)}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "account-a", Home: "synthetic-a", Enabled: true},
				{ID: "account-b", Home: "synthetic-b", Enabled: true},
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
	server := httptest.NewServer(proxy)
	defer server.Close()

	connection, reader := dialWebSocket(t, server.URL, "/backend-api/codex/responses", "")
	defer connection.Close()
	status, _, _ := readWebSocketHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("local handshake status = %d", status)
	}
	firstRequest := `{"type":"response.create","response":{}}`
	secondRequest := `{"type":"response.create","response":{"previous_response_id":"resp_1"}}`
	frames := append(maskedWebSocketFrame(9, true, "ping"), maskedWebSocketFrame(1, false, firstRequest[:len(firstRequest)/2])...)
	frames = append(frames, maskedWebSocketFrame(0, true, firstRequest[len(firstRequest)/2:])...)
	if _, err := connection.Write(frames); err != nil {
		t.Fatal(err)
	}
	if opcode, payload, err := readWebSocketTestFrame(reader); err != nil || opcode != 10 || string(payload) != "ping" {
		t.Fatalf("local pong = opcode %d, payload %q, error %v", opcode, payload, err)
	}
	if opcode, payload, err := readWebSocketTestFrame(reader); err != nil || opcode != 1 || string(payload) != `{"type":"response.completed","response":{"id":"resp_1"}}` {
		t.Fatalf("first response = opcode %d, payload %q, error %v", opcode, payload, err)
	}
	if _, err := connection.Write(maskedWebSocketFrame(1, true, secondRequest)); err != nil {
		t.Fatal(err)
	}
	if opcode, payload, err := readWebSocketTestFrame(reader); err != nil || opcode != 1 || string(payload) != `{"type":"response.completed","response":{"id":"resp_2"}}` {
		t.Fatalf("second response = opcode %d, payload %q, error %v", opcode, payload, err)
	}
	gateway.mu.Lock()
	accounts := append([]string(nil), gateway.accounts...)
	messages := append([]string(nil), gateway.messages...)
	sessionIDs := append([]uint64(nil), gateway.sessionIDs...)
	gateway.mu.Unlock()
	if len(accounts) != 2 || accounts[0] != accounts[1] || messages[0] != firstRequest || messages[1] != secondRequest {
		t.Fatalf("routed messages = accounts %#v, bodies %#v", accounts, messages)
	}
	if len(sessionIDs) != 2 || sessionIDs[0] == 0 || sessionIDs[0] != sessionIDs[1] {
		t.Fatalf("websocket session IDs = %#v", sessionIDs)
	}
	connection.Close()
	select {
	case closed := <-gateway.closedSessions:
		if closed != sessionIDs[0] {
			t.Fatalf("closed websocket session = %d, want %d", closed, sessionIDs[0])
		}
	case <-time.After(time.Second):
		t.Fatal("websocket session was not closed when client disconnected")
	}
}

type websocketMessageTestGateway struct {
	mu             sync.Mutex
	accounts       []string
	messages       []string
	sessionIDs     []uint64
	closedSessions chan uint64
}

func (gateway *websocketMessageTestGateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("unexpected HTTP request")
}

func (gateway *websocketMessageTestGateway) ExecuteWebSocket(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("unexpected raw websocket route")
}

func (gateway *websocketMessageTestGateway) ExecuteWebSocketMessage(_ context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	index := len(gateway.messages) + 1
	gateway.accounts = append(gateway.accounts, account.ID)
	gateway.messages = append(gateway.messages, string(request.Body))
	gateway.sessionIDs = append(gateway.sessionIDs, request.WebSocketSessionID)
	payload := []byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_%d"}}`, index))
	var frames bytes.Buffer
	if err := websocketframe.WriteFrame(&frames, 1, payload, false); err != nil {
		return nil, err
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(frames.Bytes())),
		WebSocketFrames: true, FirstEventCommitted: true,
		WebSocketResponseID: fmt.Sprintf("resp_%d", index),
	}, nil
}

func (gateway *websocketMessageTestGateway) CloseWebSocketSession(sessionID uint64) {
	gateway.closedSessions <- sessionID
}
