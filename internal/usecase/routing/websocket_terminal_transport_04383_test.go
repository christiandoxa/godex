package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04383DeltaOnlyTransportFailureReturnsTerminal503WithoutReplay(t *testing.T) {
	for _, readyAlternate := range []bool{false, true} {
		t.Run(map[bool]string{false: "single owner", true: "ready alternate"}[readyAlternate], func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{{
				StatusCode: http.StatusBadGateway, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader("")), WebSocketFrames: true,
				PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true},
			}}}
			accounts := websocketQuotaAccounts()
			if !readyAlternate {
				accounts = accounts[:1]
			}
			router, err := NewRouter(Config{Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{turn: "turn-a", session: "session-a"}, router.now()); err != nil {
				t.Fatal(err)
			}
			request := websocketDispatchRequest(`{"type":"response.create","session_id":"session-a","client_metadata":{"x-codex-turn-state":"turn-a"},"input":[{"type":"function_call_output","call_id":"call-large","output":"done"}]}`, 843303)
			request.Header.Set("x-codex-turn-state", "turn-a")
			forwarded, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer forwarded.Close()
			assertTerminalTransport50304383(t, forwarded.Result.Response)
			owner, err := router.affinity.owner(t.Context(), affinityKeys{turn: "turn-a", session: "session-a"}, router.now())
			if err != nil || owner != websocketQuotaOwnerA || len(gateway.accounts) != 1 || gateway.accounts[0] != websocketQuotaOwnerA || !forwarded.Result.Failed {
				t.Fatalf("terminal failure changed ownership or replayed: owner=%q attempts=%v failed=%t err=%v", owner, gateway.accounts, forwarded.Result.Failed, err)
			}
		})
	}
}

func TestProdex04383FreshExhaustedWebSocketTransportFailureReturnsTerminal503(t *testing.T) {
	pending := &pendingResponse{accountID: "synthetic-a", transport: true, response: &proxymodel.Response{
		StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")), WebSocketFrames: true,
		PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true},
	}}
	forwarded, err := finishFresh(&pending)
	if err != nil {
		t.Fatal(err)
	}
	defer forwarded.Response.Body.Close()
	assertTerminalTransport50304383(t, forwarded.Response)
}

func TestProdex04383TerminalTransportFinalizationPreservesOtherResponses(t *testing.T) {
	for _, name := range []string{"committed", "realtime", "http", "application error"} {
		t.Run(name, func(t *testing.T) {
			response := &proxymodel.Response{StatusCode: 502, Body: io.NopCloser(strings.NewReader("original")), WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true}}
			switch name {
			case "committed":
				response.FirstEventCommitted = true
			case "realtime":
				response.WebSocketRealtimeDuplex = true
			case "http":
				response.WebSocketFrames = false
			case "application error":
				response.PrecommitFailure.Transport = false
			}
			pending := &pendingResponse{response: response}
			pending.finishWebSocketTransportFailure()
			defer pending.close()
			if pending.response != response {
				t.Fatal("rewrote a response outside the terminal precommit transport boundary")
			}
		})
	}
}

func assertTerminalTransport50304383(t *testing.T, response *proxymodel.Response) {
	t.Helper()
	if response == nil || response.Body == nil {
		t.Fatal("missing terminal transport response")
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Type   string                         `json:"type"`
		Status int                            `json:"status"`
		Error  struct{ Code, Message string } `json:"error"`
	}
	if err := json.Unmarshal(payload, &event); err != nil || response.StatusCode != 503 || event.Type != "error" || event.Status != 503 || event.Error.Code != "service_unavailable" || event.Error.Message != upstreamTransportFailureMessage {
		t.Fatalf("empty or incorrect terminal transport frame: status=%d body=%q err=%v", response.StatusCode, payload, err)
	}
}
