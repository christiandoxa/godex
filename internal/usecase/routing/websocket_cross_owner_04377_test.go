package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04377WebSocketSessionOnlyPreviousResponseNotFoundRotatesOwner(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{
			StatusCode:          http.StatusOK,
			Body:                io.NopCloser(strings.NewReader("first-frame")),
			WebSocketFrames:     true,
			FirstEventCommitted: true,
		},
		{
			StatusCode:       http.StatusOK,
			Body:             io.NopCloser(strings.NewReader("previous-response-not-found")),
			WebSocketFrames:  true,
			PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found"},
		},
		{
			StatusCode:          http.StatusOK,
			Body:                io.NopCloser(strings.NewReader("second-frame")),
			WebSocketFrames:     true,
			FirstEventCommitted: true,
		},
	}}
	router := newWebSocketMessageRouter(t, gateway)

	first, err := router.Forward(context.Background(), websocketMessageRequest(
		`{"type":"response.create","session_id":"session-owner"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if first.Result.AccountID != "account-a" {
		t.Fatalf("first owner = %q, want account-a", first.Result.AccountID)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	next, err := router.Forward(context.Background(), websocketMessageRequest(
		`{"type":"response.create","session_id":"session-owner"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if next.Result.AccountID != "account-b" || next.Result.Response.PrecommitFailure != nil ||
		strings.Join(gateway.accounts, ",") != "account-a,account-a,account-b" {
		t.Fatalf("session-only continuation = account=%q failed=%t attempts=%v", next.Result.AccountID, next.Result.Failed, gateway.accounts)
	}
}

func TestProdex04377WebSocketTurnOnlyPreviousResponseNotFoundRotatesOwner(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{
			StatusCode:          http.StatusOK,
			Body:                io.NopCloser(strings.NewReader("first-frame")),
			WebSocketFrames:     true,
			FirstEventCommitted: true,
			WebSocketResponseID: "resp-first",
			WebSocketTurnState:  "turn-owner",
		},
		{
			StatusCode:       http.StatusOK,
			Body:             io.NopCloser(strings.NewReader("previous-response-not-found")),
			WebSocketFrames:  true,
			PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found"},
		},
		{
			StatusCode:          http.StatusOK,
			Body:                io.NopCloser(strings.NewReader("second-frame")),
			WebSocketFrames:     true,
			FirstEventCommitted: true,
		},
	}}
	router := newWebSocketMessageRouter(t, gateway)

	first, err := router.Forward(context.Background(), websocketMessageRequest(
		`{"type":"response.create"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if first.Result.AccountID != "account-a" {
		t.Fatalf("first owner = %q, want account-a", first.Result.AccountID)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	continuation := websocketMessageRequest(`{"type":"response.create"}`)
	continuation.Header.Set("x-codex-turn-state", "turn-owner")
	next, err := router.Forward(context.Background(), continuation)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if next.Result.AccountID != "account-b" || next.Result.Response.PrecommitFailure != nil ||
		strings.Join(gateway.accounts, ",") != "account-a,account-a,account-b" {
		t.Fatalf("turn-only continuation = account=%q failed=%t attempts=%v", next.Result.AccountID, next.Result.Failed, gateway.accounts)
	}
}

func TestProdex04377WebSocketPreviousResponseNotFoundWithoutTurnOrSessionStaysFresh(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{{
		StatusCode:       http.StatusOK,
		Body:             io.NopCloser(strings.NewReader("previous-response-not-found")),
		WebSocketFrames:  true,
		PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found"},
	}}}
	router := newWebSocketMessageRouter(t, gateway)

	result, err := router.Forward(context.Background(), websocketMessageRequest(
		`{"type":"response.create"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	if result.Result.AccountID != "account-a" || !result.Result.Failed ||
		strings.Join(gateway.accounts, ",") != "account-a" {
		t.Fatalf("unbound previous-response failure = account=%q failed=%t attempts=%v",
			result.Result.AccountID, result.Result.Failed, gateway.accounts)
	}
}
