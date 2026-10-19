package routing

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04383CompactTurnRecoveryKeepsWireAndReleasesOnlyFailedIdentity(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		for _, session := range []string{"", "sess-compact"} {
			name := map[bool]string{false: "memory", true: "durable"}[persisted] + "/" + session
			t.Run(name, func(t *testing.T) {
				gateway := &prodex04356Gateway{steps: []prodex04356Step{
					{account: websocketQuotaOwnerA, err: &url.Error{Op: "Post", Err: io.EOF}},
					{account: websocketQuotaOwnerB, status: http.StatusOK, body: `{"object":"response.compaction","output":[]}`},
				}}
				config := Config{Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if persisted {
					config.Bindings = routingrepo.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{turn: "turn-compact", session: "sess-compact", previous: "resp-unrelated"}, router.now()); err != nil {
					t.Fatal(err)
				}
				body := `{"client_metadata":{"x-codex-turn-state":"turn-compact","sequence":18446744073709551615},"input":[{"role":"user","content":"compact","sequence":9007199254740993}]}`
				request := proxymodel.Request{
					Method: http.MethodPost, Path: "/responses/compact", Body: []byte(body),
					Header:         http.Header{"X-Codex-Turn-State": {"turn-compact"}},
					QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindCompact},
				}
				if session != "" {
					request.Header.Set("session_id", session)
				}
				exchange, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer exchange.Close()
				if strings.Join(gateway.calls, ",") != websocketQuotaOwnerA+","+websocketQuotaOwnerB || exchange.Result.AccountID != websocketQuotaOwnerB || exchange.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("compact turn recovery calls=%v owner=%q status=%d", gateway.calls, exchange.Result.AccountID, exchange.Result.Response.StatusCode)
				}
				for _, sent := range gateway.requests {
					if string(sent.Body) != body || sent.Header.Get("x-codex-turn-state") != "turn-compact" {
						t.Fatal("compact recovery changed the opaque request")
					}
				}
				store := router.affinity
				if persisted {
					store = newAffinityStore()
					store.repository = config.Bindings
					if err := store.loadContinuationStatuses(t.Context(), router.now()); err != nil {
						t.Fatal(err)
					}
				}
				owner, err := store.owner(t.Context(), affinityKeys{turn: "turn-compact"}, router.now())
				if err != nil || owner != "" || !store.deadTurnStateContext(t.Context(), "turn-compact", router.now()) {
					t.Fatalf("failed compact token was rebound: owner=%q err=%v", owner, err)
				}
				owner, err = store.owner(t.Context(), affinityKeys{previous: "resp-unrelated"}, router.now())
				if err != nil || owner != websocketQuotaOwnerA {
					t.Fatalf("unrelated response changed: owner=%q err=%v", owner, err)
				}
				store.mu.Lock()
				err = store.loadMissingLocked(t.Context(), affinityKeys{session: "sess-compact"}, router.now())
				owner = store.values[affinityDigest("session", "sess-compact")].accountID
				store.mu.Unlock()
				wantSession := websocketQuotaOwnerA
				if session != "" {
					wantSession = routingentity.ConflictAccountID
				}
				if err != nil || owner != wantSession {
					t.Fatalf("compact session owner=%q err=%v want=%q", owner, err, wantSession)
				}
			})
		}
	}
}
