package routing

import (
	"context"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged Prodex 0.438.3: a precommit transport failure on a turn-state-only
// continuation with reconstructed FULL history may release the failed owner
// and safely replay on a different ready profile. It must never carry the
// stale turn binding in the HTTP header or client_metadata of the replay.
func TestProdex04383TransportBoundTurnStateFullHistoryRotatesToReadyProfile(t *testing.T) {
	failed := func() *proxymodel.Response {
		return &proxymodel.Response{
			StatusCode: http.StatusBadGateway, Header: make(http.Header),
			Body:            io.NopCloser(strings.NewReader("synthetic transport error")),
			WebSocketFrames: true, WebSocketTurnState: "turn-transport-a",
			PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true},
		}
	}
	first := websocketCommittedTestResponse("resp-initial")
	first.WebSocketTurnState = "turn-transport-a"
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		first, failed(), websocketCommittedTestResponse("resp-recovered"),
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	seed, err := router.Forward(t.Context(), websocketDispatchRequest(
		`{"type":"response.create","session_id":"session-transport","input":[{"type":"message","role":"user","content":"first"}]}`, 831))
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	request := websocketDispatchRequest(`{"type":"response.create","session_id":"session-transport","client_metadata":{"x-codex-turn-state":"turn-transport-a","sequence":18446744073709551615},"input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"new question","sequence":9007199254740993}]}`, 831)
	request.Header.Set("x-codex-turn-state", "turn-transport-a")
	result, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatalf("full-history transport fallback unavailable: %v, attempts=%v", err, gateway.accounts)
	}
	defer result.Close()
	if result.Result.AccountID != websocketQuotaOwnerB ||
		result.Result.Response.WebSocketResponseID != "resp-recovered" {
		t.Fatalf("replayed on wrong owner or no recovered result: account=%q response=%+v attempts=%v", result.Result.AccountID, result.Result.Response, gateway.accounts)
	}
	if strings.Join(gateway.accounts, ",") != strings.Join([]string{websocketQuotaOwnerA, websocketQuotaOwnerA, websocketQuotaOwnerB}, ",") {
		t.Fatalf("transport failure rotated too early/late: accounts=%v", gateway.accounts)
	}
	replay := gateway.requests[2]
	if replay.Header.Get("x-codex-turn-state") != "" || strings.Contains(string(replay.Body), "turn-transport-a") ||
		!strings.Contains(string(replay.Body), "new question") ||
		!strings.Contains(string(replay.Body), "9007199254740993") ||
		!strings.Contains(string(replay.Body), "18446744073709551615") || replay.FirstEventRetryUsed {
		t.Fatalf("full-history replay leaked failed turn state or damaged history: header=%v body=%s retryUsed=%v", replay.Header, replay.Body, replay.FirstEventRetryUsed)
	}
}

func TestProdex04383TransportTurnStateDeltaOnlyRemainsBound(t *testing.T) {
	failed := &proxymodel.Response{
		StatusCode: http.StatusBadGateway, Header: make(http.Header),
		Body:            io.NopCloser(strings.NewReader("synthetic transport error")),
		WebSocketFrames: true, WebSocketTurnState: "turn-delta-a",
		PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true},
	}
	first := websocketCommittedTestResponse("resp-initial")
	first.WebSocketTurnState = "turn-delta-a"
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{first, failed}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	seed, err := router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create","session_id":"delta-session"}`, 832))
	if err != nil {
		t.Fatal(err)
	}
	_ = seed.Close()
	request := websocketDispatchRequest(`{"type":"response.create","session_id":"delta-session","x-codex-turn-state":"turn-delta-a","input":[{"type":"custom_tool_call_output","call_id":"call-old","output":"done"}]}`, 832)
	request.Header.Set("x-codex-turn-state", "turn-delta-a")
	result, err := router.Forward(t.Context(), request)
	if err == nil && result.Result.Response != nil {
		defer result.Close()
	}
	if len(gateway.accounts) != 2 || gateway.accounts[1] != websocketQuotaOwnerA {
		t.Fatalf("delta-only request inappropriately replayed on alternative owner: accounts=%v err=%v", gateway.accounts, err)
	}
}

func TestProdex04383TransportFullHistoryRejectsUnsafeRotationVariants(t *testing.T) {
	for _, tc := range []struct {
		name          string
		body          string
		policy        proxymodel.WebSocketPolicy
		failure       responseOutcome
		owned         bool
		otherOwner    bool
		ready         bool
		compact       bool
		retryUsed     bool
		cachedBlocked bool
	}{
		{name: "delta-only tool output", body: `{"type":"response.create","x-codex-turn-state":"turn-bound","input":[{"type":"custom_tool_call_output","call_id":"c","output":"done"}]}`, failure: responseOutcome{kind: responseRetry, transport: true}, owned: true, ready: true},
		{name: "previous-response dependency", body: `{"type":"response.create","previous_response_id":"resp-old","x-codex-turn-state":"turn-bound","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, failure: responseOutcome{kind: responseRetry, transport: true}, owned: true, ready: true},
		{name: "strict compact followup", body: `{"type":"response.create","x-codex-turn-state":"turn-bound","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, failure: responseOutcome{kind: responseRetry, transport: true}, owned: true, ready: true, compact: true},
		{name: "previous retry budget consumed", body: `{"type":"response.create","x-codex-turn-state":"turn-bound","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, failure: responseOutcome{kind: responseRetry, transport: true}, owned: true, ready: true, retryUsed: true},
		{name: "cached negative quota evidence", body: `{"type":"response.create","x-codex-turn-state":"turn-bound","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, failure: responseOutcome{kind: responseRetry, transport: true}, owned: true, ready: true, cachedBlocked: true},
		{name: "realtime duplex", body: `{"type":"response.create","x-codex-turn-state":"turn-bound","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, policy: proxymodel.WebSocketPolicy{RealtimeDuplex: true}, failure: responseOutcome{kind: responseRetry, transport: true}, owned: true, ready: true},
		{name: "committed output is not retry", body: `{"type":"response.create","x-codex-turn-state":"turn-bound","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, failure: responseOutcome{kind: responsePass, transport: true}, owned: true, ready: true},
		{name: "application failure without quota or transport", body: `{"type":"response.create","x-codex-turn-state":"turn-bound","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, failure: responseOutcome{kind: responseRetry}, owned: true, ready: true},
		{name: "another profile's turn is never transferred", body: `{"type":"response.create","x-codex-turn-state":"turn-bound","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, failure: responseOutcome{kind: responseRetry, transport: true}, owned: true, otherOwner: true, ready: true},
		{name: "no ready alternate", body: `{"type":"response.create","x-codex-turn-state":"turn-bound","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, failure: responseOutcome{kind: responseRetry, transport: true}, owned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := websocketQuotaAccounts()
			if !tc.ready {
				accounts = accounts[:1]
			}
			router, err := NewRouter(Config{
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			bindingOwner := websocketQuotaOwnerA
			if tc.otherOwner {
				bindingOwner = websocketQuotaOwnerB
			}
			if tc.owned {
				if err := router.affinity.remember(t.Context(), bindingOwner,
					affinityKeys{turn: "turn-bound"}, router.now()); err != nil {
					t.Fatal(err)
				}
			}
			request := websocketDispatchRequest(tc.body, 843)
			request.Header.Set("x-codex-turn-state", "turn-bound")
			request.WebSocketPolicy = tc.policy
			if tc.compact {
				request.QuotaSelection.RouteKind = quotamodel.RouteKindCompact
			}
			request.FirstEventRetryUsed = tc.retryUsed
			if tc.cachedBlocked {
				now := router.now()
				router.storeQuotaCheck(quotaCheckKey{
					accountID: websocketQuotaOwnerB, selection: request.QuotaSelection,
				}, quotaCheck{checkedAt: now, retryAt: now.Add(time.Hour)})
			}
			response := &proxymodel.Response{
				StatusCode:       http.StatusBadGateway,
				Body:             io.NopCloser(strings.NewReader("uncommitted")),
				PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true},
			}
			pending := &pendingResponse{response: response}
			result, handled, err := router.recoverWebSocketTurnStateFullHistory04383(
				t.Context(), request, accounts, accounts[0], routedWebSocketRequestMetadata(request), tc.failure, pending,
			)
			if err != nil {
				t.Fatal(err)
			}
			if handled || result.Response != nil {
				if result.Response != nil && result.Response.Body != nil {
					_ = result.Response.Body.Close()
				}
				t.Fatalf("unsafe transport replay accepted: handled=%t result=%+v", handled, result)
			}
			if tc.owned {
				owner, err := router.affinity.owner(t.Context(), affinityKeys{turn: "turn-bound"}, router.now())
				if err != nil || owner != bindingOwner {
					t.Fatalf("denied replay deleted original binding: owner=%q err=%v", owner, err)
				}
			}
		})
	}
}
