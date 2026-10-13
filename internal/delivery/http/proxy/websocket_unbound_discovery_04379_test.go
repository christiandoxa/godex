package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type unboundWebsocketDiscoveryGateway04379 struct {
	mu       sync.Mutex
	accounts []string
	requests []proxymodel.Request
}

func (*unboundWebsocketDiscoveryGateway04379) Execute(
	context.Context, proxymodel.Request, proxymodel.Account,
) (*proxymodel.Response, error) {
	panic("unbound WebSocket must not use the HTTP transport")
}

func (gateway *unboundWebsocketDiscoveryGateway04379) ExecuteWebSocketMessage(
	_ context.Context, request proxymodel.Request, account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	gateway.accounts = append(gateway.accounts, account.ID)
	gateway.requests = append(gateway.requests, request)
	gateway.mu.Unlock()
	var wire bytes.Buffer
	if strings.Contains(string(request.Body), "resp-tool-unknown") {
		failure := []byte(`{"type":"response.failed","response":{"error":{"code":"previous_response_not_found"}}}`)
		_ = websocketframe.WriteFrame(&wire, 1, failure, false)
		return &proxymodel.Response{
			StatusCode: http.StatusOK, Header: make(http.Header),
			Body:             io.NopCloser(bytes.NewReader(wire.Bytes())),
			WebSocketFrames:  true,
			PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found"},
		}, nil
	}
	success := []byte(`{"type":"response.completed","response":{"id":"resp-after-stale"}}`)
	_ = websocketframe.WriteFrame(&wire, 1, success, false)
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body:            io.NopCloser(bytes.NewReader(wire.Bytes())),
		WebSocketFrames: true, FirstEventCommitted: true,
		WebSocketResponseID: "resp-after-stale",
	}, nil
}

func (*unboundWebsocketDiscoveryGateway04379) CloseWebSocketSession(uint64) {}

func TestProdex04379UnboundWebSocketToolOutputNeverCrossProfileOnMissingPrevious(t *testing.T) {
	gateway := &unboundWebsocketDiscoveryGateway04379{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway:          gateway,
		PreferredAccount: websocketHardOwner04379,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: websocketHardOwner04379, Home: "/synthetic/a", Enabled: true},
				{ID: websocketSoftOwner04379, Home: "/synthetic/b", Enabled: true},
			}, nil
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
	client, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/prodex/responses")
	defer client.Close()
	if status, _ := readResponsesPublicHandshake(t, reader); status != http.StatusSwitchingProtocols {
		t.Fatalf("local handshake = %d", status)
	}
	send := func(payload string) string {
		t.Helper()
		if _, err := client.Write(protocolClientFrame(1, true, []byte(payload), true)); err != nil {
			t.Fatal(err)
		}
		return readResponsesPublicTextFrame(t, reader)
	}
	const tool = `{"type":"response.create","previous_response_id":"resp-tool-unknown","session_id":"sess-unbound","input":[{"type":"function_call_output","call_id":"call-42","output":"done"}]}`
	signal := send(tool)
	if !strings.Contains(signal, "stale_continuation") || strings.Contains(signal, "call-42") {
		t.Fatalf("missing previous-response tool output was replayed or leaked: %s", signal)
	}
	gateway.mu.Lock()
	owners := append([]string(nil), gateway.accounts...)
	requests := append([]proxymodel.Request(nil), gateway.requests...)
	gateway.mu.Unlock()
	// The native locked-affinity policy allows the initial attempt and
	// three short *same-owner* retries (75/200/500ms), but never switches
	// account or drops the tool-output continuation.
	if len(owners) != 4 || len(requests) != 4 {
		t.Fatalf("opaque tool context made %d attempts on owners %v; want four bounded owner retries", len(requests), owners)
	}
	for index, sent := range requests {
		if owners[index] != websocketHardOwner04379 ||
			string(sent.Body) != tool ||
			sent.QuotaSelection.RouteKind != quotamodel.RouteKindWebSocket {
			t.Fatalf("attempt %d rewrote opaque tool context or changed owner: owner=%q body=%q route=%q",
				index, owners[index], sent.Body, sent.QuotaSelection.RouteKind)
		}
	}
	if completed := send(`{"type":"response.create","input":[{"role":"user","content":"start over"}]}`); !strings.Contains(completed, "resp-after-stale") {
		t.Fatalf("local session unusable after one failed discovery: %s", completed)
	}
}
