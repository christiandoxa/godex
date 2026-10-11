package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04372WebSocketQuotaPositiveBusyFallbackSignalsFullContext(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-main"),
		websocketQuotaFailureResponse(),
		websocketCommittedTestResponse("resp-second"),
	}}
	var waits int
	var router *Router
	var err error
	router, err = NewRouter(Config{
		Gateway:                  gateway,
		PreferredAccount:         websocketQuotaOwnerA,
		ProfileInflightHardLimit: 2,
		ProfileInflightWait: func(context.Context, <-chan struct{}, time.Duration) error {
			waits++
			router.mu.Lock()
			router.inflight[websocketQuotaOwnerB] = 0
			router.mu.Unlock()
			return nil
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return websocketQuotaAccounts(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := router.Forward(t.Context(), websocketDispatchRequest(
		`{"type":"response.create","session_id":"session-busy","input":[{"type":"message","role":"user","content":"turn one"}]}`,
		201,
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	router.mu.Lock()
	router.inflight[websocketQuotaOwnerB] = 2
	router.mu.Unlock()

	continuation, err := router.Forward(t.Context(), websocketDispatchRequest(
		`{"type":"response.create","session_id":"session-busy","previous_response_id":"resp-main","input":[{"type":"custom_tool_call_output","call_id":"call-1","output":"done"}]}`,
		201,
	))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(continuation.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if continuation.Result.Response.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(body), "previous_response_not_found") {
		t.Fatalf("busy quota-positive fallback leaked quota failure: account=%q status=%d body=%s attempts=%v",
			continuation.Result.AccountID, continuation.Result.Response.StatusCode, body, gateway.accounts)
	}
	if err := continuation.Close(); err != nil {
		t.Fatal(err)
	}
	replay, err := router.Forward(t.Context(), websocketDispatchRequest(
		`{"type":"response.create","session_id":"session-busy","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"turn one result"},{"type":"message","role":"user","content":"turn two"}]}`,
		202,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if replay.Result.AccountID != websocketQuotaOwnerB ||
		replay.Result.Response.WebSocketResponseID != "resp-second" ||
		waits != 1 ||
		strings.Join(gateway.accounts, ",") != websocketQuotaOwnerA+","+websocketQuotaOwnerA+","+websocketQuotaOwnerB {
		t.Fatalf("full-context replay handoff = account=%q response=%q waits=%d attempts=%v",
			replay.Result.AccountID, replay.Result.Response.WebSocketResponseID, waits, gateway.accounts)
	}
}

func TestProdex04372WebSocketCommittedQuotaDoesNotReplay(t *testing.T) {
	committedQuota := &proxymodel.Response{
		StatusCode:          http.StatusOK,
		Header:              make(http.Header),
		Body:                io.NopCloser(strings.NewReader("quota-after-output")),
		WebSocketFrames:     true,
		FirstEventCommitted: true,
		PrecommitFailure:    &proxymodel.PrecommitFailure{Code: "insufficient_quota"},
	}
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-main"),
		committedQuota,
		websocketCommittedTestResponse("must-not-replay"),
	}}
	router, err := NewRouter(Config{
		Gateway:          gateway,
		PreferredAccount: websocketQuotaOwnerA,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return websocketQuotaAccounts(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := router.Forward(t.Context(), websocketDispatchRequest(
		`{"type":"response.create","session_id":"session-committed","input":[{"type":"message","role":"user","content":"turn one"}]}`,
		203,
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := router.Forward(t.Context(), websocketDispatchRequest(
		`{"type":"response.create","session_id":"session-committed","previous_response_id":"resp-main","input":[{"type":"custom_tool_call_output","call_id":"call-1","output":"done"}]}`,
		203,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	body, err := io.ReadAll(result.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.AccountID != websocketQuotaOwnerA ||
		!result.Result.Response.FirstEventCommitted ||
		!strings.Contains(string(body), "quota-after-output") ||
		strings.Join(gateway.accounts, ",") != websocketQuotaOwnerA+","+websocketQuotaOwnerA {
		t.Fatalf("committed quota replayed = account=%q body=%q attempts=%v",
			result.Result.AccountID, body, gateway.accounts)
	}
}

func TestProdex04372WebSocketQuotaRecoverySeparatesCooldownFromExhaustion(t *testing.T) {
	now := time.Unix(4_000, 0)
	router, err := NewRouter(Config{
		Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return websocketQuotaAccounts(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := proxymodel.Request{QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindWebSocket}}
	router.transportBackoffs[routeHealthKey{accountID: websocketQuotaOwnerB, route: "websocket"}] = routingentity.TransportBackoff{
		AccountID: websocketQuotaOwnerB, Route: "websocket", UntilUnix: now.Unix() + 15,
	}
	if !router.websocketQuotaFullContextFallbackRecoverable(request, websocketQuotaAccounts(), proxymodel.Account{ID: websocketQuotaOwnerA}) {
		t.Fatal("temporary transport cooldown was treated as exhausted quota")
	}
	router.quarantineAccount(websocketQuotaOwnerB, time.Minute)
	if router.websocketQuotaFullContextFallbackRecoverable(request, websocketQuotaAccounts(), proxymodel.Account{ID: websocketQuotaOwnerA}) {
		t.Fatal("retry quarantine manufactured a recoverable quota destination")
	}
	router.clearQuarantine(websocketQuotaOwnerB)
	router.storeQuotaCheck(quotaCheckKey{
		accountID: websocketQuotaOwnerB, selection: request.QuotaSelection,
	}, quotaCheck{checkedAt: now, retryAt: now.Add(time.Minute)})
	if router.websocketQuotaFullContextFallbackRecoverable(request, websocketQuotaAccounts(), proxymodel.Account{ID: websocketQuotaOwnerA}) {
		t.Fatal("cached quota exhaustion was treated as recoverable")
	}
}
