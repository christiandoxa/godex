package routing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04355PreSendQuotaBlockedHardAffinitySignalsFullContextReplay(t *testing.T) {
	now := time.Unix(1_000, 0)
	accounts := websocketQuotaAccounts()
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-presend-owner"),
		websocketCommittedTestResponse("resp-presend-replay"),
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
		Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first := mustProdex04355WebSocketForward(t, router, `{"type":"response.create","session_id":"session-presend","input":[{"type":"message","role":"user","content":"turn one"}]}`, 551)
	if first.Result.AccountID != websocketQuotaOwnerA {
		t.Fatalf("first owner = %q, want %q", first.Result.AccountID, websocketQuotaOwnerA)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	accounts[0].EligibleAfter = now.Add(time.Hour)
	signal := mustProdex04355WebSocketForward(t, router, `{"type":"response.create","session_id":"session-presend","previous_response_id":"resp-presend-owner","input":[{"type":"custom_tool_call_output","call_id":"call-551","output":"done"}]}`, 551)
	assertProdex04355FullContextSignal(t, signal, websocketQuotaOwnerA)
	if err := signal.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(gateway.accounts, ","); got != websocketQuotaOwnerA {
		t.Fatalf("pre-send block reached upstream: attempts=%v", gateway.accounts)
	}
	assertProdex04355AffinityReleased(t, router, "resp-presend-owner", "session-presend")

	replay := mustProdex04355WebSocketForward(t, router, `{"type":"response.create","session_id":"session-presend","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"turn one result"},{"type":"message","role":"user","content":"turn two"}]}`, 552)
	defer replay.Close()
	if replay.Result.AccountID != websocketQuotaOwnerB || replay.Result.Response.WebSocketResponseID != "resp-presend-replay" {
		t.Fatalf("full-context replay = account=%q response=%q", replay.Result.AccountID, replay.Result.Response.WebSocketResponseID)
	}
	if got := strings.Join(gateway.accounts, ","); got != websocketQuotaOwnerA+","+websocketQuotaOwnerB {
		t.Fatalf("full-context replay attempts = %v", gateway.accounts)
	}
}

func TestProdex04355HardAffinityQuotaSignalAcceptsLastChanceFallback(t *testing.T) {
	now := time.Unix(2_000, 0)
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-last-owner"),
		websocketQuotaFailureResponse(),
		websocketCommittedTestResponse("resp-last-replay"),
	}}
	var waits []time.Duration
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
		Now: func() time.Time { return now },
		Wait: func(context.Context, time.Duration) error {
			waits = append(waits, time.Second)
			return errors.New("full-context last-chance replay must not wait")
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return websocketQuotaAccounts(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first := mustProdex04355WebSocketForward(t, router, `{"type":"response.create","session_id":"session-last","input":[{"type":"message","role":"user","content":"turn one"}]}`, 553)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount(websocketQuotaOwnerB, time.Minute)

	signal := mustProdex04355WebSocketForward(t, router, `{"type":"response.create","session_id":"session-last","previous_response_id":"resp-last-owner","input":[{"type":"custom_tool_call_output","call_id":"call-553","output":"done"}]}`, 553)
	assertProdex04355FullContextSignal(t, signal, websocketQuotaOwnerA)
	if err := signal.Close(); err != nil {
		t.Fatal(err)
	}
	assertProdex04355AffinityReleased(t, router, "resp-last-owner", "session-last")

	replay := mustProdex04355WebSocketForward(t, router, `{"type":"response.create","session_id":"session-last","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"turn one result"},{"type":"message","role":"user","content":"turn two"}]}`, 554)
	defer replay.Close()
	if replay.Result.AccountID != websocketQuotaOwnerB || replay.Result.Response.WebSocketResponseID != "resp-last-replay" {
		t.Fatalf("last-chance replay = account=%q response=%q", replay.Result.AccountID, replay.Result.Response.WebSocketResponseID)
	}
	if len(waits) != 0 {
		t.Fatalf("last-chance replay waited: %v", waits)
	}
	if got := strings.Join(gateway.accounts, ","); got != websocketQuotaOwnerA+","+websocketQuotaOwnerA+","+websocketQuotaOwnerB {
		t.Fatalf("last-chance attempts = %v", gateway.accounts)
	}
}

func TestProdex04355PreSendFullContextSignalRejectsHardCappedFallback(t *testing.T) {
	now := time.Unix(3_000, 0)
	accounts := websocketQuotaAccounts()
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-hard-cap-owner"),
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
		Now: func() time.Time { return now }, ProfileInflightHardLimit: 8,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first := mustProdex04355WebSocketForward(t, router, `{"type":"response.create","session_id":"session-hard-cap-4355","input":[{"type":"message","role":"user","content":"turn one"}]}`, 555)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	accounts[0].EligibleAfter = now.Add(time.Hour)
	router.mu.Lock()
	router.inflight[websocketQuotaOwnerB] = 8
	router.mu.Unlock()

	_, err = router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create","session_id":"session-hard-cap-4355","previous_response_id":"resp-hard-cap-owner","input":[{"type":"custom_tool_call_output","call_id":"call-555","output":"done"}]}`, 555))
	var routeErr *proxymodel.Error
	if !errors.As(err, &routeErr) || routeErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("hard-capped fallback error = %v, want 503", err)
	}
	owner, ownerErr := router.affinity.owner(t.Context(), affinityKeys{session: "session-hard-cap-4355"}, router.now())
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	if owner != websocketQuotaOwnerA {
		t.Fatalf("hard-capped fallback released owner = %q", owner)
	}
	if got := strings.Join(gateway.accounts, ","); got != websocketQuotaOwnerA {
		t.Fatalf("hard-capped pre-send block reached upstream: %v", gateway.accounts)
	}
}

func mustProdex04355WebSocketForward(t *testing.T, router *Router, body string, sessionID uint64) *Exchange {
	t.Helper()
	exchange, err := router.Forward(t.Context(), websocketDispatchRequest(body, sessionID))
	if err != nil {
		t.Fatalf("websocket forward failed: %v", err)
	}
	return exchange
}

func assertProdex04355FullContextSignal(t *testing.T, exchange *Exchange, owner string) {
	t.Helper()
	body, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	exchange.Result.Response.Body = io.NopCloser(strings.NewReader(string(body)))
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("signal is not JSON: %v body=%s", err, body)
	}
	errorValue, _ := payload["error"].(map[string]any)
	if exchange.Result.AccountID != owner || exchange.Result.Response.StatusCode != 400 ||
		websocketJSONText(errorValue["code"]) != "previous_response_not_found" ||
		websocketJSONText(errorValue["message"]) != "Previous response was not found. Retrying the full request." ||
		strings.Contains(string(body), "service_unavailable") || strings.Contains(string(body), "insufficient_quota") {
		t.Fatalf("full-context signal = account=%q status=%d body=%s", exchange.Result.AccountID, exchange.Result.Response.StatusCode, body)
	}
}

func assertProdex04355AffinityReleased(t *testing.T, router *Router, previous, session string) {
	t.Helper()
	previousOwner, err := router.affinity.owner(t.Context(), affinityKeys{previous: previous}, router.now())
	if err != nil {
		t.Fatal(err)
	}
	sessionOwner, err := router.affinity.owner(t.Context(), affinityKeys{session: session}, router.now())
	if err != nil {
		t.Fatal(err)
	}
	if previousOwner != "" || sessionOwner != "" {
		t.Fatalf("full-context signal retained affinity: previous=%q session=%q", previousOwner, sessionOwner)
	}
}
