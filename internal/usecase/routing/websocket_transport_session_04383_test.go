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

func TestProdex04383TransportTurnRecoveryRetainsAccompanyingSessionOwner(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "durable"}[persisted], func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")), WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true}},
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
			now := router.now()
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{turn: "turn-a", session: "session-a", previous: "resp-original"}, now); err != nil {
				t.Fatal(err)
			}
			request := websocketDispatchRequest(`{"type":"response.create","session_id":"session-a","client_metadata":{"x-codex-turn-state":"turn-a"},"input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, 843304)
			request.Header.Set("x-codex-turn-state", "turn-a")
			forwarded, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer forwarded.Close()
			if forwarded.Result.AccountID != websocketQuotaOwnerB || strings.Join(gateway.accounts, ",") != websocketQuotaOwnerA+","+websocketQuotaOwnerB {
				t.Fatalf("full-history transport sequence changed: %v", gateway.accounts)
			}
			store := router.affinity
			if persisted {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			sessionKey := affinityDigest("session", "session-a")
			store.mu.Lock()
			if err := store.loadMissingLocked(t.Context(), affinityKeys{session: "session-a"}, router.now()); err != nil {
				store.mu.Unlock()
				t.Fatal(err)
			}
			owner := store.values[sessionKey].accountID
			store.mu.Unlock()
			if owner != routingentity.ConflictAccountID {
				t.Fatalf("hard-turn replay released accompanying session and silently reassigned it: owner=%q", owner)
			}
			turnOwner, err := store.owner(t.Context(), affinityKeys{turn: "turn-a"}, router.now())
			if err != nil || turnOwner != "" {
				t.Fatalf("failed turn survived: owner=%q err=%v", turnOwner, err)
			}
			original, err := store.owner(t.Context(), affinityKeys{previous: "resp-original"}, router.now())
			if err != nil || original != websocketQuotaOwnerA {
				t.Fatalf("unrelated original response owner changed: owner=%q err=%v", original, err)
			}
		})
	}
}
