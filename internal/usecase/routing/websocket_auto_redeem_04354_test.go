package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestWebSocketFreshConnectQuotaAutoRedeemsSameProfileOnce(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"insufficient_quota"}}`)),
		},
		websocketCommittedTestResponse("resp-redeemed"),
	}}
	redeemer := &fakeRoutingRedeemer{accountID: "account-a", redeemed: true}
	router := newWebSocketAutoRedeemRouter(t, gateway, redeemer)

	exchange, err := router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create"}`, 71))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-a" ||
		exchange.Result.Response.WebSocketResponseID != "resp-redeemed" ||
		strings.Join(gateway.accounts, ",") != "account-a,account-a" ||
		redeemer.calls != 1 ||
		len(redeemer.preferred) != 1 ||
		redeemer.preferred[0] != "account-a" {
		t.Fatalf("connect auto-redeem = result=%#v accounts=%v redeemer=%#v",
			exchange.Result, gateway.accounts, redeemer)
	}
}

func TestWebSocketMessageQuotaDoesNotAutoRedeemAndRotatesFreshProfile(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		{
			StatusCode:       http.StatusOK,
			Header:           make(http.Header),
			Body:             io.NopCloser(strings.NewReader("quota-frame")),
			WebSocketFrames:  true,
			PrecommitFailure: &proxymodel.PrecommitFailure{Code: "insufficient_quota"},
		},
		websocketCommittedTestResponse("resp-b"),
	}}
	redeemer := &fakeRoutingRedeemer{accountID: "account-a", redeemed: true}
	router := newWebSocketAutoRedeemRouter(t, gateway, redeemer)

	exchange, err := router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create"}`, 72))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-b" ||
		strings.Join(gateway.accounts, ",") != "account-a,account-b" ||
		redeemer.calls != 0 {
		t.Fatalf("message quota fallback = result=%#v accounts=%v redeemer=%#v",
			exchange.Result, gateway.accounts, redeemer)
	}
}

func TestWebSocketSessionContextDisablesConnectQuotaAutoRedeem(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"insufficient_quota"}}`)),
		},
		websocketCommittedTestResponse("resp-b"),
	}}
	redeemer := &fakeRoutingRedeemer{accountID: "account-a", redeemed: true}
	router := newWebSocketAutoRedeemRouter(t, gateway, redeemer)

	exchange, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","session_id":"session-1"}`, 73),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-b" ||
		strings.Join(gateway.accounts, ",") != "account-a,account-b" ||
		redeemer.calls != 0 {
		t.Fatalf("session connect quota = result=%#v accounts=%v redeemer=%#v",
			exchange.Result, gateway.accounts, redeemer)
	}
}

func newWebSocketAutoRedeemRouter(
	t *testing.T,
	gateway *websocketDispatchGateway,
	redeemer *fakeRoutingRedeemer,
) *Router {
	t.Helper()
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a",
		AutoRedeem: true, Redeemer: redeemer,
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
	return router
}
