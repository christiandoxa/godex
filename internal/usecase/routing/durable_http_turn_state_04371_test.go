package routing

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04371ResponsesTurnStateSurvivesRouterRestart(t *testing.T) {
	profileHome := t.TempDir()
	if err := os.Chmod(profileHome, 0o700); err != nil {
		t.Fatal(err)
	}
	stateHome := t.TempDir()
	const accountID = "0123456789abcdef0123456789abcdef"
	const responseID = "resp-http-turn-state"
	const turnState = "turn-http-durable"
	now := time.Now().Truncate(time.Second)
	accounts := func(context.Context) ([]proxymodel.Account, error) {
		return []proxymodel.Account{{ID: accountID, Home: profileHome, Enabled: true}}, nil
	}
	response := responseRecoveryReply{
		status: http.StatusOK, contentType: "application/json",
		body: `{"object":"response","id":"` + responseID + `"}`,
	}
	responseHeader := func() http.Header {
		return http.Header{"Content-Type": []string{"application/json"}, "X-Codex-Turn-State": []string{turnState}}
	}

	firstGateway := &responseRecoveryGateway{replies: []responseRecoveryReply{response}}
	firstRouter, err := NewRouter(Config{
		Gateway: firstGateway, Bindings: routingrepo.NewStore(stateHome), Now: func() time.Time { return now },
		Accounts: accounts,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstGateway.headers = []http.Header{responseHeader()}
	first, err := firstRouter.Forward(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/responses", Header: make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	turnPath := filepath.Join(profileHome, ".godex-turn-state", affinityDigest("previous", responseID)+".json")
	if _, err := os.Stat(turnPath); err != nil {
		t.Fatalf("committed HTTP turn state was not persisted: %v", err)
	}

	secondGateway := &responseRecoveryGateway{replies: []responseRecoveryReply{response}}
	secondRouter, err := NewRouter(Config{
		Gateway: secondGateway, Bindings: routingrepo.NewStore(stateHome), Now: func() time.Time { return now },
		Accounts: accounts,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondGateway.headers = []http.Header{responseHeader()}
	second, err := secondRouter.Forward(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/responses", Header: make(http.Header),
		Body:           []byte(`{"previous_response_id":"` + responseID + `"}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if got := secondGateway.requests[0].Header.Get("x-codex-turn-state"); got != turnState {
		t.Fatalf("restarted HTTP request turn state = %q, want %q; body=%s", got, turnState, strings.TrimSpace(string(secondGateway.requests[0].Body)))
	}
}
