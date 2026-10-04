package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type invalidPreviousPublicGateway struct {
	mu       sync.Mutex
	calls    int
	accounts []string
}

func (*invalidPreviousPublicGateway) Execute(
	context.Context,
	proxymodel.Request,
	proxymodel.Account,
) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("standard execute must not handle invalid previous websocket test")
}

func (gateway *invalidPreviousPublicGateway) ExecuteWebSocketMessage(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	gateway.calls++
	call := gateway.calls
	gateway.accounts = append(gateway.accounts, account.ID)
	gateway.mu.Unlock()
	switch call {
	case 1:
		return invalidPreviousPublicCommitted("resp-owner"), nil
	case 2:
		return invalidPreviousPublicFailure(), nil
	default:
		return invalidPreviousPublicCommitted("resp-recovered"), nil
	}
}

func invalidPreviousPublicCommitted(responseID string) *proxymodel.Response {
	payload := []byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"%s"}}`, responseID))
	var frames bytes.Buffer
	_ = websocketframe.WriteFrame(&frames, 1, payload, false)
	return &proxymodel.Response{
		StatusCode:          http.StatusOK,
		Header:              make(http.Header),
		Body:                io.NopCloser(bytes.NewReader(frames.Bytes())),
		WebSocketFrames:     true,
		FirstEventCommitted: true,
		WebSocketResponseID: responseID,
	}
}

func invalidPreviousPublicFailure() *proxymodel.Response {
	payload := []byte("{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Invalid `previous_response_id`.\"}}")
	var frames bytes.Buffer
	_ = websocketframe.WriteFrame(&frames, 1, payload, false)
	return &proxymodel.Response{
		StatusCode:      http.StatusOK,
		Header:          make(http.Header),
		Body:            io.NopCloser(bytes.NewReader(frames.Bytes())),
		WebSocketFrames: true,
		PrecommitFailure: &proxymodel.PrecommitFailure{
			Code:                      "previous_response_not_found",
			InvalidPreviousResponseID: true,
		},
	}
}

func TestPublicResponsesWebSocketSignalsInvalidPreviousAndAcceptsFullContextReplay(t *testing.T) {
	gateway := &invalidPreviousPublicGateway{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway:          gateway,
		PreferredAccount: "account-a",
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
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/codex/responses")
	defer connection.Close()
	status, _ := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d", status)
	}
	send := func(message string) string {
		t.Helper()
		if _, err := connection.Write(protocolClientFrame(1, true, []byte(message), true)); err != nil {
			t.Fatal(err)
		}
		return readResponsesPublicTextFrame(t, reader)
	}

	if first := send(`{"type":"response.create","session_id":"session-chain","input":[{"type":"message","role":"user","content":"turn one"}]}`); !strings.Contains(first, `"id":"resp-owner"`) {
		t.Fatalf("first response = %s", first)
	}
	signal := send(`{"type":"response.create","session_id":"session-chain","previous_response_id":"resp-owner","input":[{"type":"message","role":"user","content":"turn two"}]}`)
	var value map[string]any
	if err := json.Unmarshal([]byte(signal), &value); err != nil {
		t.Fatal(err)
	}
	errorValue, _ := value["error"].(map[string]any)
	if int(value["status"].(float64)) != http.StatusBadRequest ||
		errorValue["code"] != "previous_response_not_found" ||
		errorValue["message"] != "Invalid `previous_response_id`." ||
		strings.Contains(signal, "stale_continuation") {
		t.Fatalf("retry signal = %s", signal)
	}

	recovered := send(`{"type":"response.create","session_id":"session-chain","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"turn one result"},{"type":"message","role":"user","content":"turn two"}]}`)
	if !strings.Contains(recovered, `"id":"resp-recovered"`) {
		t.Fatalf("recovered response = %s", recovered)
	}
	gateway.mu.Lock()
	accounts := append([]string(nil), gateway.accounts...)
	gateway.mu.Unlock()
	if strings.Join(accounts, ",") != "account-a,account-a,account-a" {
		t.Fatalf("owner trace = %v", accounts)
	}
}
