package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const prodex04357TurnState = "turn-post-compaction-quota"

func prodex04357DeadTurnState(store *affinityStore, value string, now time.Time) bool {
	key := affinityDigest("turn", value)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneContinuationStatusesLocked(now)
	if _, live := store.values[key]; live {
		return false
	}
	status, ok := store.statuses[key]
	return ok && status.kind == "turn_state" && status.state == continuationDead
}

func prodex04357MarkDeadTurnState(store *affinityStore, value string, now time.Time) {
	key := affinityDigest("turn", value)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.markContinuationDeadLocked("turn_state", key, now)
}

func prodex04357FullHistoryBody() []byte {
	return []byte(`{"model":"gpt-6-luna","input":[{"type":"message","role":"user","content":"compacted history"},{"type":"message","role":"assistant","content":"completed work"},{"type":"message","role":"user","content":"continue"}],"client_metadata":{"x-codex-turn-state":"turn-post-compaction-quota"}}`)
}

func prodex04357UnsafeBody() []byte {
	return []byte(`{"model":"gpt-6-luna","input":[{"type":"message","role":"user","content":"continue"}],"client_metadata":{"x-codex-turn-state":"turn-post-compaction-quota"}}`)
}

func prodex04357ResponsesRequest(body []byte) proxymodel.Request {
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	header.Set("X-Codex-Turn-State", prodex04357TurnState)
	return proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/codex/responses",
		Header: header, Body: append([]byte(nil), body...),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}
}

func assertProdex04357TurnStateScrubbed(t *testing.T, request proxymodel.Request) {
	t.Helper()
	if got := strings.TrimSpace(request.Header.Get("x-codex-turn-state")); got != "" {
		t.Fatalf("replay forwarded dead turn-state header %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(request.Body, &body); err != nil {
		t.Fatalf("decode replay body: %v body=%s", err, request.Body)
	}
	if _, found := body["x-codex-turn-state"]; found {
		t.Fatalf("replay forwarded top-level turn-state: %s", request.Body)
	}
	if metadata, ok := body["client_metadata"].(map[string]any); ok {
		if _, found := metadata["x-codex-turn-state"]; found {
			t.Fatalf("replay forwarded client_metadata turn-state: %s", request.Body)
		}
	}
}

func TestProdex04357ResponsesPostCompactionQuotaReplaysWithoutTurnState(t *testing.T) {
	now := time.Unix(5_700_001, 0)
	gateway := &prodex04356Gateway{steps: []prodex04356Step{
		{account: "account-a", status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota","message":"usage limit"}}`},
		{account: "account-b", status: http.StatusOK, body: `{"id":"resp-second"}`},
	}}
	router := newProdex04356Router(t, gateway, prodex04356Accounts())
	router.now = func() time.Time { return now }
	if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{turn: prodex04357TurnState}, now); err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(t.Context(), prodex04357ResponsesRequest(prodex04357FullHistoryBody()))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	body, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	observed := append(append([]byte(nil), exchange.Result.Prefix...), body...)
	if exchange.Result.AccountID != "account-b" || exchange.Result.Failed ||
		exchange.Result.Response.StatusCode != http.StatusOK || !strings.Contains(string(observed), "resp-second") {
		t.Fatalf("post-compaction replay = owner:%q failed:%t status:%d payload:%s",
			exchange.Result.AccountID, exchange.Result.Failed, exchange.Result.Response.StatusCode, observed)
	}
	if !reflect.DeepEqual(gateway.calls, []string{"account-a", "account-b"}) {
		t.Fatalf("post-compaction attempts = %v", gateway.calls)
	}
	assertProdex04357TurnStateScrubbed(t, gateway.requests[1])
	if owner, err := router.affinity.owner(t.Context(), affinityKeys{turn: prodex04357TurnState}, now); err != nil || owner != "" {
		t.Fatalf("dead turn-state owner = %q err=%v", owner, err)
	}
	if !prodex04357DeadTurnState(router.affinity, prodex04357TurnState, now) {
		t.Fatal("quota-released turn-state was not marked dead")
	}
}

func TestProdex04357WebSocketPostCompactionQuotaReplaysWithoutTurnState(t *testing.T) {
	now := time.Unix(5_700_002, 0)
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketQuotaFailureResponse(),
		websocketCommittedTestResponse("resp-second"),
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
		Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return websocketQuotaAccounts(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{turn: prodex04357TurnState}, now); err != nil {
		t.Fatal(err)
	}
	request := websocketDispatchRequest(string(prodex04357FullHistoryBody()), 5700)
	request.Header.Set("X-Codex-Turn-State", prodex04357TurnState)
	request.QuotaSelection = quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}

	exchange, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != websocketQuotaOwnerB ||
		exchange.Result.Response.WebSocketResponseID != "resp-second" ||
		exchange.Result.Response.PrecommitFailure != nil {
		t.Fatalf("websocket post-compaction replay = owner:%q response:%q failure:%#v",
			exchange.Result.AccountID, exchange.Result.Response.WebSocketResponseID, exchange.Result.Response.PrecommitFailure)
	}
	if !reflect.DeepEqual(gateway.accounts, []string{websocketQuotaOwnerA, websocketQuotaOwnerB}) {
		t.Fatalf("websocket attempts = %v", gateway.accounts)
	}
	assertProdex04357TurnStateScrubbed(t, gateway.requests[1])
	if !prodex04357DeadTurnState(router.affinity, prodex04357TurnState, now) {
		t.Fatal("websocket quota-released turn-state was not marked dead")
	}
}

func TestProdex04357TurnStateQuotaWithoutFullHistoryFailsClosed(t *testing.T) {
	now := time.Unix(5_700_003, 0)
	gateway := &prodex04356Gateway{steps: []prodex04356Step{{
		account: "account-a", status: http.StatusForbidden,
		body: `{"error":{"code":"insufficient_quota","message":"usage limit"}}`,
	}}}
	router := newProdex04356Router(t, gateway, prodex04356Accounts())
	router.now = func() time.Time { return now }
	if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{turn: prodex04357TurnState}, now); err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), prodex04357ResponsesRequest(prodex04357UnsafeBody()))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-a" || !exchange.Result.Failed ||
		exchange.Result.Response.StatusCode != http.StatusForbidden || len(gateway.calls) != 1 ||
		!strings.Contains(string(exchange.Result.Prefix), "insufficient_quota") {
		t.Fatalf("unsafe turn-state quota did not fail closed: owner:%q failed:%t status:%d calls:%v prefix:%s",
			exchange.Result.AccountID, exchange.Result.Failed, exchange.Result.Response.StatusCode, gateway.calls, exchange.Result.Prefix)
	}
	if owner, err := router.affinity.owner(t.Context(), affinityKeys{turn: prodex04357TurnState}, now); err != nil || owner != "account-a" {
		t.Fatalf("unsafe replay released turn-state owner = %q err=%v", owner, err)
	}
}

func TestProdex04357DeadTurnStateIsScrubbedBeforeNextSelection(t *testing.T) {
	now := time.Unix(5_700_004, 0)
	gateway := &prodex04356Gateway{steps: []prodex04356Step{
		{account: "account-a", status: http.StatusOK, body: `{"id":"resp-next"}`},
	}}
	router := newProdex04356Router(t, gateway, prodex04356Accounts())
	router.now = func() time.Time { return now }
	prodex04357MarkDeadTurnState(router.affinity, prodex04357TurnState, now)

	request := prodex04357ResponsesRequest(prodex04357FullHistoryBody())
	exchange, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if len(gateway.requests) != 1 {
		t.Fatalf("next request attempts = %d", len(gateway.requests))
	}
	assertProdex04357TurnStateScrubbed(t, gateway.requests[0])
}
