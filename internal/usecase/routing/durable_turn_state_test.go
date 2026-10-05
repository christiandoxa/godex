package routing

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestWebSocketTurnStateRestoresAfterRouterRestart(t *testing.T) {
	profileHome := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(profileHome, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stateHome := t.TempDir()
	owner := strings.Repeat("a", 32)
	other := strings.Repeat("b", 32)
	accounts := []proxymodel.Account{
		{ID: owner, Home: profileHome, Enabled: true},
		{ID: other, Home: t.TempDir(), Enabled: true},
	}
	bindings := routingrepo.NewStore(stateHome)
	const responseID = "resp_durable_turn_state"
	const turnState = "opaque-durable-turn-state"

	firstGateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: responseID, WebSocketTurnState: turnState},
	}}
	firstRouter := newDurableTurnStateTestRouter(t, firstGateway, bindings, accounts, owner)
	first, err := firstRouter.Forward(context.Background(), websocketMessageRequest(`{"type":"response.create","response":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if first.Result.AccountID != owner {
		t.Fatalf("first owner = %q, want %q", first.Result.AccountID, owner)
	}
	_ = first.Close()

	secondGateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("next-frame")), WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: "resp_next"},
	}}
	secondRouter := newDurableTurnStateTestRouter(t, secondGateway, bindings, accounts, other)
	second, err := secondRouter.Forward(context.Background(), websocketMessageRequest(
		`{"type":"response.create","response":{"previous_response_id":"`+responseID+`"}}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Result.AccountID != owner || len(secondGateway.requests) != 1 ||
		secondGateway.requests[0].Header.Get("x-codex-turn-state") != turnState {
		t.Fatalf("restored owner/header = %q/%q", second.Result.AccountID, secondGateway.requests[0].Header.Get("x-codex-turn-state"))
	}

	contents, err := os.ReadFile(filepath.Join(stateHome, "routing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), responseID) || strings.Contains(string(contents), turnState) {
		t.Fatal("routing metadata contains raw continuation state")
	}
}

func newDurableTurnStateTestRouter(
	t *testing.T,
	gateway *websocketMessageRoutingGateway,
	bindings *routingrepo.Store,
	accounts []proxymodel.Account,
	preferred string,
) *Router {
	t.Helper()
	router, err := NewRouter(Config{
		Gateway: gateway, Bindings: bindings, PreferredAccount: preferred,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return accounts, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}
