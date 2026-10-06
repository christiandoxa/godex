package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestFreshWebSocketMessageWaitsForRetryableBackoffWithoutTransientFailure(t *testing.T) {
	now := time.Unix(200, 0)
	account := proxymodel.Account{
		ID: "account-a", Home: "synthetic-a", Enabled: true,
		Provider: proxymodel.Provider{Kind: "openai"},
	}
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{{
		StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("recovered-frame")),
		WebSocketFrames: true, FirstEventCommitted: true,
	}}}
	waits := 0
	accountLoads := 0
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Wait: func(_ context.Context, delay time.Duration) error {
			waits++
			now = now.Add(delay)
			return nil
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			accountLoads++
			return []proxymodel.Account{account}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindWebSocket}
	router.storeQuotaCheck(quotaCheckKey{accountID: account.ID, selection: selection}, quotaCheck{
		checkedAt: now, ready: true,
	})
	router.quarantineAccount(account.ID, 3*time.Second)

	request := websocketMessageRequest(`{"type":"response.create","response":{}}`)
	request.QuotaSelection = selection
	exchange, err := router.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if waits != 1 || accountLoads != 2 || len(gateway.accounts) != 1 || gateway.accounts[0] != account.ID ||
		exchange.Result.AccountID != account.ID || exchange.Result.Failed {
		t.Fatalf("fresh websocket recovery: waits=%d account_loads=%d accounts=%v result=%#v", waits, accountLoads, gateway.accounts, exchange.Result)
	}
}

func TestFreshWebSocketMessageReselectsAccountsAfterRecoveryWait(t *testing.T) {
	now := time.Unix(300, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "synthetic-a", Enabled: true, Provider: proxymodel.Provider{Kind: "openai"}},
		{ID: "account-b", Home: "synthetic-b", Enabled: false, Provider: proxymodel.Provider{Kind: "openai"}},
	}
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{{
		StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("reselected-frame")),
		WebSocketFrames: true, FirstEventCommitted: true,
	}}}
	waits := 0
	accountLoads := 0
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Wait: func(_ context.Context, delay time.Duration) error {
			waits++
			now = now.Add(delay)
			accounts[0].Enabled = false
			accounts[1].Enabled = true
			return nil
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			accountLoads++
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindWebSocket}
	for _, account := range accounts {
		router.storeQuotaCheck(quotaCheckKey{accountID: account.ID, selection: selection}, quotaCheck{
			checkedAt: now, ready: true,
		})
	}
	router.quarantineAccount("account-a", 3*time.Second)
	request := websocketMessageRequest(`{"type":"response.create","response":{}}`)
	request.QuotaSelection = selection

	exchange, err := router.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if waits != 1 || accountLoads != 2 || len(gateway.accounts) != 1 || gateway.accounts[0] != "account-b" ||
		exchange.Result.AccountID != "account-b" || exchange.Result.Failed {
		t.Fatalf("fresh websocket re-selection: waits=%d account_loads=%d accounts=%v result=%#v", waits, accountLoads, gateway.accounts, exchange.Result)
	}
}

func TestWebSocketHardContinuationDoesNotRotateDuringRetryBackoff(t *testing.T) {
	now := time.Unix(400, 0)
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{{
		StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("first-frame")),
		WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_owner",
	}}}
	router, err := NewRouter(Config{
		Gateway: gateway, Now: func() time.Time { return now },
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
	firstRequest := websocketMessageRequest(`{"type":"response.create","session_id":"session-a","response":{}}`)
	firstRequest.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
	first, err := router.Forward(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	owner := first.Result.AccountID
	_ = first.Close()
	router.quarantineAccount(owner, 3*time.Second)

	continuationRequest := websocketMessageRequest(`{"type":"response.create","session_id":"session-a","response":{"previous_response_id":"resp_owner"}}`)
	continuationRequest.QuotaSelection.RouteKind = quotamodel.RouteKindWebSocket
	_, err = router.Forward(context.Background(), continuationRequest)
	var routeError *proxymodel.Error
	if !errors.As(err, &routeError) || routeError.StatusCode != http.StatusServiceUnavailable ||
		len(gateway.accounts) != 1 || gateway.accounts[0] != owner {
		t.Fatalf("hard continuation recovery error=%v accounts=%v owner=%q", err, gateway.accounts, owner)
	}
}

func TestWebSocketPreSendQuotaBlockedContinuationSignalsReplayAndRebinds(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "synthetic-a", Enabled: true},
		{ID: "account-b", Home: "synthetic-b", Enabled: true},
	}
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("first-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_owner"},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("replayed-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_replayed"},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a", Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := router.Forward(context.Background(), websocketMessageRequest(
		`{"type":"response.create","session_id":"session-a","response":{}}`,
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
		`{"type":"response.create","session_id":"session-a","response":{"previous_response_id":"resp_owner"}}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	payload, readErr := io.ReadAll(continuation.Result.Response.Body)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if continuation.Result.Response.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(payload), "previous_response_not_found") ||
		len(gateway.requests) != 1 {
		t.Fatalf("pre-send replay signal = status:%d body:%s calls:%d",
			continuation.Result.Response.StatusCode, payload, len(gateway.requests))
	}
	_ = continuation.Close()

	replayBody := `{"type":"response.create","session_id":"session-a","input":[{"role":"user","content":"full context"}]}`
	replay, err := router.Forward(context.Background(), websocketMessageRequest(replayBody))
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if replay.Result.AccountID != "account-b" || len(gateway.requests) != 2 ||
		gateway.accounts[0] != "account-a" || gateway.accounts[1] != "account-b" ||
		string(gateway.requests[1].Body) != replayBody {
		t.Fatalf("full-context replay owner = %q, accounts = %v, requests = %#v", replay.Result.AccountID, gateway.accounts, gateway.requests)
	}
}
