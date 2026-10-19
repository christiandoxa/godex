package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04383TurnOnlyMissingResponseKeepsOriginalHardOwner(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "durable"}[persisted], func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("missing")), WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found"}},
				websocketCommittedTestResponse("resp-recovered"),
			}}
			config := Config{Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
			if persisted {
				config.Bindings = routingrepository.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{turn: "turn-a", session: "session-a", previous: "resp-original"}, router.now()); err != nil {
				t.Fatal(err)
			}
			request := websocketDispatchRequest(`{"type":"response.create","session_id":"session-a","client_metadata":{"x-codex-turn-state":"turn-a"},"input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, 843306)
			request.Header.Set("x-codex-turn-state", "turn-a")
			forwarded, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer forwarded.Close()
			if forwarded.Result.AccountID != websocketQuotaOwnerB || strings.Join(gateway.accounts, ",") != websocketQuotaOwnerA+","+websocketQuotaOwnerB {
				t.Fatalf("missing-response attempt sequence changed: %v", gateway.accounts)
			}
			if gateway.requests[1].Header.Get("x-codex-turn-state") != "turn-a" || string(gateway.requests[1].Body) != string(request.Body) {
				t.Fatal("missing-response recovery changed the opaque request")
			}
			store := router.affinity
			if persisted {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			keys := affinityKeys{turn: "turn-a", session: "session-a", previous: "resp-original"}
			store.mu.Lock()
			if err := store.loadMissingLocked(t.Context(), keys, router.now()); err != nil {
				store.mu.Unlock()
				t.Fatal(err)
			}
			session := store.values[affinityDigest("session", "session-a")].accountID
			store.mu.Unlock()
			turn, turnErr := store.owner(t.Context(), affinityKeys{turn: "turn-a"}, router.now())
			original, originalErr := store.owner(t.Context(), affinityKeys{previous: "resp-original"}, router.now())
			recovered, recoveredErr := store.owner(t.Context(), affinityKeys{previous: "resp-recovered"}, router.now())
			if session != routingentity.ConflictAccountID || turn != websocketQuotaOwnerA || original != websocketQuotaOwnerA || recovered != websocketQuotaOwnerB || turnErr != nil || originalErr != nil || recoveredErr != nil {
				t.Fatalf("missing-response durable owners changed: session=%q turn=%q original=%q recovered=%q errors=%v/%v/%v", session, turn, original, recovered, turnErr, originalErr, recoveredErr)
			}
		})
	}
}
