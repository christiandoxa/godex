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
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type quotaFullContextPublicGateway struct {
	mu        sync.Mutex
	responses []*proxymodel.Response
	accounts  []string
}

func (gateway *quotaFullContextPublicGateway) Execute(
	context.Context,
	proxymodel.Request,
	proxymodel.Account,
) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("standard execute must not handle websocket quota fallback")
}

func (gateway *quotaFullContextPublicGateway) ExecuteWebSocketMessage(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.accounts = append(gateway.accounts, account.ID)
	if len(gateway.responses) == 0 {
		return nil, fmt.Errorf("missing websocket quota fixture")
	}
	response := gateway.responses[0]
	gateway.responses = gateway.responses[1:]
	return response, nil
}

func (gateway *quotaFullContextPublicGateway) CloseWebSocketSession(uint64) {}

func TestPublicResponsesWebSocketQuotaSignalsFullContextAndReplaysOnFallback(t *testing.T) {
	gateway := &quotaFullContextPublicGateway{responses: []*proxymodel.Response{
		quotaPublicCommittedResponse("resp-main"),
		quotaPublicFailureResponse(),
		quotaPublicCommittedResponse("resp-second"),
	}}
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

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/prodex/responses")
	status, _ := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		_ = connection.Close()
		t.Fatalf("handshake status = %d", status)
	}
	sendText := func(message string) string {
		t.Helper()
		if _, err := connection.Write(protocolClientFrame(1, true, []byte(message), true)); err != nil {
			t.Fatal(err)
		}
		return readResponsesPublicTextFrame(t, reader)
	}
	first := sendText("{\"type\":\"response.create\",\"session_id\":\"session-quota\",\"input\":[{\"type\":\"message\",\"role\":\"user\",\"content\":\"turn one\"}]}")
	if !strings.Contains(first, "\"id\":\"resp-main\"") {
		_ = connection.Close()
		t.Fatalf("first response = %s", first)
	}
	signal := sendText("{\"type\":\"response.create\",\"session_id\":\"session-quota\",\"previous_response_id\":\"resp-main\",\"input\":[{\"type\":\"custom_tool_call_output\",\"call_id\":\"call-1\",\"output\":\"done\"}]}")
	var signalValue map[string]any
	if err := json.Unmarshal([]byte(signal), &signalValue); err != nil {
		_ = connection.Close()
		t.Fatal(err)
	}
	signalError, _ := signalValue["error"].(map[string]any)
	if signalValue["type"] != "error" ||
		int(signalValue["status"].(float64)) != http.StatusBadRequest ||
		signalError["code"] != "previous_response_not_found" ||
		signalError["message"] != "Previous response was not found. Retrying the full request." ||
		strings.Contains(signal, "insufficient_quota") {
		_ = connection.Close()
		t.Fatalf("quota full-context signal = %s", signal)
	}
	if _, err := connection.Write(protocolClientFrame(8, true, []byte{0x03, 0xe8}, true)); err != nil {
		_ = connection.Close()
		t.Fatal(err)
	}
	_ = connection.Close()

	connection, reader = dialResponsesPublicWebSocket(t, server.URL, "/backend-api/prodex/responses")
	defer connection.Close()
	status, _ = readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("replay handshake status = %d", status)
	}
	fullContext := "{\"type\":\"response.create\",\"session_id\":\"session-quota\",\"input\":[{\"type\":\"message\",\"role\":\"user\",\"content\":\"turn one\"},{\"type\":\"message\",\"role\":\"assistant\",\"content\":\"turn one result\"},{\"type\":\"message\",\"role\":\"user\",\"content\":\"turn two\"}]}"
	if _, err := connection.Write(protocolClientFrame(1, true, []byte(fullContext), true)); err != nil {
		t.Fatal(err)
	}
	replayed := readResponsesPublicTextFrame(t, reader)
	if !strings.Contains(replayed, "\"id\":\"resp-second\"") {
		t.Fatalf("full-context replay = %s", replayed)
	}

	gateway.mu.Lock()
	accounts := append([]string(nil), gateway.accounts...)
	gateway.mu.Unlock()
	if strings.Join(accounts, ",") != "account-a,account-a,account-b" {
		t.Fatalf("quota replay accounts = %v", accounts)
	}
}

func quotaPublicCommittedResponse(responseID string) *proxymodel.Response {
	payload := []byte(fmt.Sprintf("{\"type\":\"response.completed\",\"response\":{\"id\":%q}}", responseID))
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

func quotaPublicFailureResponse() *proxymodel.Response {
	var frames bytes.Buffer
	_ = websocketframe.WriteFrame(
		&frames,
		1,
		[]byte("{\"type\":\"response.failed\",\"error\":{\"code\":\"insufficient_quota\",\"message\":\"usage limit\"}}"),
		false,
	)
	return &proxymodel.Response{
		StatusCode:      http.StatusOK,
		Header:          make(http.Header),
		Body:            io.NopCloser(bytes.NewReader(frames.Bytes())),
		WebSocketFrames: true,
		PrecommitFailure: &proxymodel.PrecommitFailure{
			Code: "insufficient_quota",
		},
	}
}

func TestPublicResponsesWebSocketProdex04355PreSendQuotaSignalsFullContext(t *testing.T) {
	now := time.Unix(4_355, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: true},
	}
	gateway := &quotaFullContextPublicGateway{responses: []*proxymodel.Response{
		quotaPublicCommittedResponse("resp-presend-main"),
		quotaPublicCommittedResponse("resp-presend-second"),
	}}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway, PreferredAccount: "account-a", Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
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

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/prodex/responses")
	status, _ := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		_ = connection.Close()
		t.Fatalf("handshake status = %d", status)
	}
	sendText := func(message string) string {
		t.Helper()
		if _, err := connection.Write(protocolClientFrame(1, true, []byte(message), true)); err != nil {
			t.Fatal(err)
		}
		return readResponsesPublicTextFrame(t, reader)
	}
	first := sendText(`{"type":"response.create","session_id":"session-presend-public","input":[{"type":"message","role":"user","content":"turn one"}]}`)
	if !strings.Contains(first, `"id":"resp-presend-main"`) {
		_ = connection.Close()
		t.Fatalf("first response = %s", first)
	}

	accounts[0].EligibleAfter = now.Add(time.Hour)
	signal := sendText(`{"type":"response.create","session_id":"session-presend-public","previous_response_id":"resp-presend-main","input":[{"type":"custom_tool_call_output","call_id":"call-public-4355","output":"done"}]}`)
	var signalValue map[string]any
	if err := json.Unmarshal([]byte(signal), &signalValue); err != nil {
		_ = connection.Close()
		t.Fatalf("signal JSON: %v body=%s", err, signal)
	}
	signalError, _ := signalValue["error"].(map[string]any)
	if signalValue["type"] != "error" ||
		int(signalValue["status"].(float64)) != 400 ||
		signalError["code"] != "previous_response_not_found" ||
		signalError["message"] != "Previous response was not found. Retrying the full request." ||
		strings.Contains(signal, "service_unavailable") {
		_ = connection.Close()
		t.Fatalf("pre-send full-context signal = %s", signal)
	}
	gateway.mu.Lock()
	beforeReplay := append([]string(nil), gateway.accounts...)
	gateway.mu.Unlock()
	if strings.Join(beforeReplay, ",") != "account-a" {
		_ = connection.Close()
		t.Fatalf("pre-send block reached upstream: %v", beforeReplay)
	}
	if _, err := connection.Write(protocolClientFrame(8, true, []byte{0x03, 0xe8}, true)); err != nil {
		_ = connection.Close()
		t.Fatal(err)
	}
	_ = connection.Close()

	connection, reader = dialResponsesPublicWebSocket(t, server.URL, "/backend-api/prodex/responses")
	defer connection.Close()
	status, _ = readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("replay handshake status = %d", status)
	}
	fullContext := `{"type":"response.create","session_id":"session-presend-public","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"turn one result"},{"type":"message","role":"user","content":"continue"}]}`
	if _, err := connection.Write(protocolClientFrame(1, true, []byte(fullContext), true)); err != nil {
		t.Fatal(err)
	}
	replayed := readResponsesPublicTextFrame(t, reader)
	if !strings.Contains(replayed, `"id":"resp-presend-second"`) {
		t.Fatalf("full-context replay = %s", replayed)
	}
	gateway.mu.Lock()
	finalAccounts := append([]string(nil), gateway.accounts...)
	gateway.mu.Unlock()
	if strings.Join(finalAccounts, ",") != "account-a,account-b" {
		t.Fatalf("pre-send replay accounts = %v", finalAccounts)
	}
}
