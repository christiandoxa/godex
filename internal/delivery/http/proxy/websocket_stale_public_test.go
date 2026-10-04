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

type stalePublicGateway struct {
	mu       sync.Mutex
	calls    int
	accounts []string
}

func (gateway *stalePublicGateway) Execute(
	context.Context,
	proxymodel.Request,
	proxymodel.Account,
) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("standard execute must not handle websocket stale test")
}

func (gateway *stalePublicGateway) ExecuteWebSocketMessage(
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
		return stalePublicCommittedResponse("resp-owner"), nil
	case 2:
		payload := []byte(`{"type":"response.failed","status":400,"response":{"error":{"code":"previous_response_not_found","message":"missing"}}}`)
		return stalePublicPreviousFailure(payload), nil
	default:
		return stalePublicCommittedResponse("resp-after-stale"), nil
	}
}

func stalePublicCommittedResponse(responseID string) *proxymodel.Response {
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

func stalePublicPreviousFailure(payload []byte) *proxymodel.Response {
	var frames bytes.Buffer
	_ = websocketframe.WriteFrame(&frames, 1, payload, false)
	return &proxymodel.Response{
		StatusCode:       http.StatusOK,
		Header:           make(http.Header),
		Body:             io.NopCloser(bytes.NewReader(frames.Bytes())),
		WebSocketFrames:  true,
		PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found"},
	}
}

func TestPublicResponsesWebSocketTranslatesKnownOwnerStaleContinuationAndContinues(t *testing.T) {
	gateway := &stalePublicGateway{}
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

	sendPublicText := func(message string) string {
		t.Helper()
		if _, err := connection.Write(protocolClientFrame(1, true, []byte(message), true)); err != nil {
			t.Fatal(err)
		}
		return readResponsesPublicTextFrame(t, reader)
	}

	if got := sendPublicText(`{"type":"response.create"}`); !strings.Contains(got, `"id":"resp-owner"`) {
		t.Fatalf("first response = %s", got)
	}
	stale := sendPublicText(`{"type":"response.create","previous_response_id":"resp-owner"}`)
	var payload map[string]any
	if err := json.Unmarshal([]byte(stale), &payload); err != nil {
		t.Fatal(err)
	}
	responseObject, _ := payload["response"].(map[string]any)
	errorObject, _ := responseObject["error"].(map[string]any)
	if payload["type"] != "response.failed" ||
		int(payload["status"].(float64)) != http.StatusConflict ||
		errorObject["code"] != "stale_continuation" ||
		strings.Contains(stale, "previous_response_not_found") {
		t.Fatalf("stale response = %s", stale)
	}

	if got := sendPublicText(`{"type":"response.create","input":[{"role":"user","content":"fresh"}]}`); !strings.Contains(got, `"id":"resp-after-stale"`) {
		t.Fatalf("post-stale response = %s", got)
	}
	gateway.mu.Lock()
	accounts := append([]string(nil), gateway.accounts...)
	gateway.mu.Unlock()
	if len(accounts) != 3 || accounts[0] != accounts[1] {
		t.Fatalf("stale continuation rotated owner: %v", accounts)
	}
}
