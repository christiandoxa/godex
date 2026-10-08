package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestWebSocketMessagesBindNestedPreviousResponseToAccount(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("first-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_first"},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("second-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_second"},
	}}
	router := newWebSocketMessageRouter(t, gateway)
	firstRequest := websocketMessageRequest(`{"type":"response.create","response":{}}`)
	firstRequest.WebSocketSessionID = 17
	first, err := router.Forward(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	firstAccount := first.Result.AccountID
	frame, err := io.ReadAll(first.Result.Response.Body)
	if err != nil || string(frame) != "first-frame" {
		t.Fatalf("first websocket body = %q, error = %v", frame, err)
	}
	_ = first.Close()

	second, err := router.Forward(context.Background(), websocketMessageRequest(`{"type":"response.create","response":{"previous_response_id":"resp_first"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Result.AccountID != firstAccount || len(gateway.requests) != 2 {
		t.Fatalf("continuation account = %q, first account = %q, requests = %d", second.Result.AccountID, firstAccount, len(gateway.requests))
	}
}

func TestWebSocketContinuationRestoresCachedTurnStateOnOwner(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("first-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_turn_owner", WebSocketTurnState: "turn-state-owner"},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("second-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_turn_next", WebSocketTurnState: "turn-state-owner"},
	}}
	router := newWebSocketMessageRouter(t, gateway)
	first, err := router.Forward(context.Background(), websocketMessageRequest(`{"type":"response.create","response":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	owner := first.Result.AccountID
	_ = first.Close()

	continuation, err := router.Forward(context.Background(), websocketMessageRequest(`{"type":"response.create","response":{"previous_response_id":"resp_turn_owner"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer continuation.Close()
	if continuation.Result.AccountID != owner || len(gateway.requests) != 2 ||
		gateway.requests[1].Header.Get("x-codex-turn-state") != "turn-state-owner" {
		t.Fatalf("restored turn state owner/header = %q/%q", continuation.Result.AccountID, gateway.requests[1].Header.Get("x-codex-turn-state"))
	}
}

func TestWebSocketContinuationCachedQuotaSignalsFullContextWhenFallbackExists(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "synthetic-a", Enabled: true},
		{ID: "account-b", Home: "synthetic-b", Enabled: true},
	}
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("first-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_owner"},
	}}
	redeemer := &fakeRoutingRedeemer{accountID: "account-b", redeemed: true}
	quota := &routingQuotaAvailabilityFake{}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a", Now: func() time.Time { return now },
		QuotaPreflight: quota,
		AutoRedeem:     true, Redeemer: redeemer,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := router.Forward(context.Background(), websocketMessageRequest(
		`{"type":"response.create","input":[{"type":"message","role":"user","content":"first"}]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if first.Result.AccountID != "account-a" {
		t.Fatalf("first response owner = %q, want account-a", first.Result.AccountID)
	}
	_ = first.Close()

	accounts[0].EligibleAfter = now.Add(time.Hour)
	continuation, err := router.Forward(context.Background(), websocketMessageRequest(
		`{"type":"response.create","response":{"previous_response_id":"resp_owner"},"input":[{"type":"message","role":"user","content":"continue"}]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer continuation.Close()
	body, err := io.ReadAll(continuation.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	owner, ownerErr := router.affinity.owner(context.Background(), affinityKeys{previous: "resp_owner"}, now)
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	if continuation.Result.Response.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(body), "previous_response_not_found") ||
		len(gateway.accounts) != 1 || gateway.accounts[0] != "account-a" ||
		redeemer.calls != 0 || len(quota.calls) != 0 || owner != "" {
		t.Fatalf("continuation status/body/accounts/owner = %d/%s/%v/%q",
			continuation.Result.Response.StatusCode, body, gateway.accounts, owner)
	}
}

func TestWebSocketPrecommitFailureRetriesOnceBeforeCommitment(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("failed-frame")), WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Code: "rate_limit_exceeded"}},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("success-frame")), WebSocketFrames: true, FirstEventCommitted: true},
	}}
	router := newWebSocketMessageRouter(t, gateway)
	exchange, err := router.Forward(context.Background(), websocketMessageRequest(`{"type":"response.create","response":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-b" || len(gateway.requests) != 2 || !gateway.requests[1].FirstEventRetryUsed {
		t.Fatalf("precommit retry = account %q, requests %#v", exchange.Result.AccountID, gateway.requests)
	}
}

func TestWebSocketReuseFailureDiscoversAndRetriesTurnStateOnSameOwner(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("first-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_owner"},
		{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("transport-failure-frame")), WebSocketFrames: true, WebSocketTurnState: "turn-state-owner", PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true}},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("retryable-error-frame")), WebSocketFrames: true, WebSocketTurnState: "turn-state-owner", PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found"}},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("recovered-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_next"},
	}}
	router := newWebSocketMessageRouter(t, gateway)
	firstRequest := websocketMessageRequest(`{"type":"response.create","response":{}}`)
	firstRequest.WebSocketSessionID = 17
	first, err := router.Forward(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	owner := first.Result.AccountID
	_ = first.Close()

	continuationRequest := websocketMessageRequest(`{"type":"response.create","response":{"previous_response_id":"resp_owner"}}`)
	continuationRequest.WebSocketSessionID = 17
	continuation, err := router.Forward(context.Background(), continuationRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer continuation.Close()
	if continuation.Result.AccountID != owner || continuation.Result.Failed || len(gateway.requests) != 4 {
		t.Fatalf("same-owner turn-state recovery failed: account = %q, failed = %t, requests = %d", continuation.Result.AccountID, continuation.Result.Failed, len(gateway.requests))
	}
	discovery, retry := gateway.requests[2], gateway.requests[3]
	if gateway.accounts[1] != owner || gateway.accounts[2] != owner || gateway.accounts[3] != owner ||
		discovery.FirstEventRetryUsed || discovery.Header.Get("x-codex-turn-state") != "" ||
		!retry.FirstEventRetryUsed || retry.Header.Get("x-codex-turn-state") != "turn-state-owner" {
		t.Fatal("turn-state retry did not stay on the owner with the retry budget marked")
	}
}

func TestWebSocketReuseTransportFailureWithoutTurnStateRetriesOwner(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("first-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_owner"},
		{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("transport-failure-frame")), WebSocketFrames: true, WebSocketReusedSession: true, PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true}},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("recovered-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_next"},
	}}
	router := newWebSocketMessageRouter(t, gateway)
	firstRequest := websocketMessageRequest(`{"type":"response.create","session_id":"session-owner","response":{}}`)
	firstRequest.WebSocketSessionID = 17
	first, err := router.Forward(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	owner := first.Result.AccountID
	_ = first.Close()

	continuationRequest := websocketMessageRequest(`{"type":"response.create","session_id":"session-owner","response":{"previous_response_id":"resp_owner"}}`)
	continuationRequest.WebSocketSessionID = 17
	continuation, err := router.Forward(context.Background(), continuationRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer continuation.Close()
	if continuation.Result.AccountID != owner || continuation.Result.Response.WebSocketResponseID != "resp_next" || len(gateway.requests) != 3 {
		t.Fatalf("reuse transport recovery = account %q response %q requests %d", continuation.Result.AccountID, continuation.Result.Response.WebSocketResponseID, len(gateway.requests))
	}
}

func TestWebSocketReuseTransportFailureRetriesSessionOwnerWithoutContinuation(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("first-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_owner"},
		{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("transport-failure-frame")), WebSocketFrames: true, WebSocketReusedSession: true, PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true}},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("recovered-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_next"},
	}}
	router := newWebSocketMessageRouter(t, gateway)
	first, err := router.Forward(context.Background(), websocketMessageRequest(`{"type":"response.create","session_id":"session-owner","response":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	owner := first.Result.AccountID
	_ = first.Close()

	request := websocketMessageRequest(`{"type":"response.create","session_id":"session-owner","input":[{"type":"message","role":"user","content":"next"}]}`)
	request.WebSocketSessionID = 17
	continuation, err := router.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer continuation.Close()
	if continuation.Result.AccountID != owner || continuation.Result.Response.WebSocketResponseID != "resp_next" || len(gateway.requests) != 3 {
		t.Fatalf("session-owner transport recovery = account %q response %q requests %d", continuation.Result.AccountID, continuation.Result.Response.WebSocketResponseID, len(gateway.requests))
	}
}

func TestWebSocketPreviousResponseRetriesFollowBoundedTurnStateSchedule(t *testing.T) {
	for _, test := range []struct {
		name          string
		continuations []*proxymodel.Response
		wantFailed    bool
	}{
		{
			name: "success after three retries",
			continuations: []*proxymodel.Response{
				websocketPreviousResponseFailure("turn-state-1"),
				websocketPreviousResponseFailure("turn-state-2"),
				websocketPreviousResponseFailure("turn-state-3"),
				{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("recovered-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_recovered"},
			},
		},
		{
			name: "stops after three retries",
			continuations: []*proxymodel.Response{
				websocketPreviousResponseFailure("turn-state-1"),
				websocketPreviousResponseFailure("turn-state-2"),
				websocketPreviousResponseFailure("turn-state-3"),
				websocketPreviousResponseFailure("turn-state-4"),
				{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("must-not-retry")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_too_late"},
			},
			wantFailed: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gateway := &websocketMessageRoutingGateway{responses: append(
				[]*proxymodel.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("first-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_retry_owner"}},
				test.continuations...,
			)}
			router := newWebSocketMessageRouter(t, gateway)
			var delays []time.Duration
			router.wait = func(_ context.Context, delay time.Duration) error {
				delays = append(delays, delay)
				return nil
			}

			first, err := router.Forward(context.Background(), websocketMessageRequest(`{"type":"response.create","response":{}}`))
			if err != nil {
				t.Fatal(err)
			}
			owner := first.Result.AccountID
			_ = first.Close()

			continuation, err := router.Forward(context.Background(), websocketMessageRequest(`{"type":"response.create","response":{"previous_response_id":"resp_retry_owner"}}`))
			if err != nil {
				t.Fatal(err)
			}
			defer continuation.Close()
			if continuation.Result.AccountID != owner || continuation.Result.Failed != test.wantFailed || len(gateway.requests) != 5 {
				t.Fatalf("continuation = account %q, failed %t, requests %d", continuation.Result.AccountID, continuation.Result.Failed, len(gateway.requests))
			}
			if !test.wantFailed && continuation.Result.Response.WebSocketResponseID != "resp_recovered" {
				t.Fatalf("recovered response id = %q", continuation.Result.Response.WebSocketResponseID)
			}
			wantDelays := []time.Duration{75 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond}
			wantTurnStates := []string{"turn-state-1", "turn-state-2", "turn-state-3"}
			if len(delays) != len(wantDelays) {
				t.Fatalf("retry delays = %v", delays)
			}
			for index, want := range wantDelays {
				if delays[index] != want {
					t.Fatalf("retry delays = %v, want %v", delays, wantDelays)
				}
				request := gateway.requests[index+2]
				if !request.FirstEventRetryUsed || request.Header.Get("x-codex-turn-state") != wantTurnStates[index] || gateway.accounts[index+2] != owner {
					t.Fatalf("retry %d did not stay on owner with returned turn state: %#v", index+1, request)
				}
			}
			if test.wantFailed && (continuation.Result.Response.PrecommitFailure == nil || !continuation.Result.Response.PrecommitFailure.StaleContinuation) {
				t.Fatal("exhausted continuation was not returned as stale")
			}
		})
	}
}

func websocketPreviousResponseFailure(turnState string) *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("retryable-error-frame")), WebSocketFrames: true,
		WebSocketTurnState: turnState, PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found"},
	}
}

func TestStaleContinuationRequiresARequestedPreviousResponse(t *testing.T) {
	for _, fixture := range []struct {
		name string
		body string
		code string
		want bool
	}{
		{name: "nested previous response", body: `{"response":{"previous_response_id":"resp_old"}}`, code: "previous_response_not_found", want: true},
		{name: "new response", body: `{"response":{}}`, code: "previous_response_not_found"},
		{name: "other upstream error", body: `{"previous_response_id":"resp_old"}`, code: "rate_limit_exceeded"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			response := &proxymodel.Response{PrecommitFailure: &proxymodel.PrecommitFailure{Code: fixture.code}}
			markStaleWebSocketContinuation(response, websocketMessageRequest(fixture.body))
			if response.PrecommitFailure.StaleContinuation != fixture.want {
				t.Fatalf("stale continuation marker = %v, want %v", response.PrecommitFailure.StaleContinuation, fixture.want)
			}
		})
	}
}

func TestWebSocketQuotaFailureRotatesWithoutAutoRedeem(t *testing.T) {
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "synthetic-a", Enabled: true},
		{ID: "account-b", Home: "synthetic-b", Enabled: true},
	}
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("quota-frame")), WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Code: "insufficient_quota"}},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("success-frame")), WebSocketFrames: true, FirstEventCommitted: true},
	}}
	redeemer := &fakeRoutingRedeemer{accountID: "account-a", redeemed: true}
	router, err := NewRouter(Config{
		Gateway: gateway, AutoRedeem: true, Redeemer: redeemer,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), websocketMessageRequest(`{"type":"response.create","response":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-b" || exchange.Result.Failed || len(gateway.requests) != 2 ||
		strings.Join(gateway.accounts, ",") != "account-a,account-b" {
		t.Fatalf("quota rotation = result=%#v accounts=%v requests=%d", exchange.Result, gateway.accounts, len(gateway.requests))
	}
	if !gateway.requests[1].FirstEventRetryUsed || redeemer.calls != 0 || len(redeemer.preferred) != 0 {
		t.Fatalf("retry/redeemer = %#v / %#v", gateway.requests[1], redeemer)
	}
}

type websocketMessageRoutingGateway struct {
	requests  []proxymodel.Request
	accounts  []string
	responses []*proxymodel.Response
}

func (gateway *websocketMessageRoutingGateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, nil
}

func (gateway *websocketMessageRoutingGateway) ExecuteWebSocketMessage(_ context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.requests = append(gateway.requests, request)
	gateway.accounts = append(gateway.accounts, account.ID)
	index := min(len(gateway.requests)-1, len(gateway.responses)-1)
	return gateway.responses[index], nil
}

func (*websocketMessageRoutingGateway) CloseWebSocketSession(uint64) {}

func newWebSocketMessageRouter(t *testing.T, gateway *websocketMessageRoutingGateway) *Router {
	t.Helper()
	router, err := NewRouter(Config{
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
	return router
}

func websocketMessageRequest(body string) proxymodel.Request {
	return proxymodel.Request{
		Method: http.MethodGet, Path: "/backend-api/codex/responses",
		Header: http.Header{"Upgrade": {"websocket"}}, Body: []byte(body), WebSocketMessage: true,
	}
}
