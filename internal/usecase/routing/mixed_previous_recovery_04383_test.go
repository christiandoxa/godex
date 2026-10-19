package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04383UnknownPreviousIDDoesNotAuthorizeOwnedTurnReset(t *testing.T) {
	for _, route := range []quotamodel.RouteKind{quotamodel.RouteKindResponses, quotamodel.RouteKindCompact} {
		name := map[quotamodel.RouteKind]string{quotamodel.RouteKindResponses: "responses", quotamodel.RouteKindCompact: "compact"}[route]
		for _, persisted := range []bool{false, true} {
			t.Run(name+"/"+map[bool]string{false: "memory", true: "durable"}[persisted], func(t *testing.T) {
				gateway := &prodex04356Gateway{steps: []prodex04356Step{
					{account: websocketQuotaOwnerA, err: &url.Error{Op: "Post", Err: io.EOF}},
					{account: websocketQuotaOwnerB, status: 200, body: `{"object":"response.compaction","output":[]}`},
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
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{turn: "turn-mixed", session: "session-mixed", previous: "resp-unrelated"}, router.now()); err != nil {
					t.Fatal(err)
				}
				body := `{"previous_response_id":"resp-unseen","session_id":"session-mixed","client_metadata":{"x-codex-turn-state":"turn-mixed","sequence":18446744073709551615},"input":[{"role":"user","content":"first","sequence":9007199254740993},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`
				path := "/responses"
				if route == quotamodel.RouteKindCompact {
					path += "/compact"
				}
				exchange, err := router.Forward(t.Context(), proxymodel.Request{
					Method: http.MethodPost, Path: path, Header: http.Header{"X-Codex-Turn-State": {"turn-mixed"}}, Body: []byte(body),
					QuotaSelection: quotamodel.Selection{RouteKind: route},
				})
				if exchange != nil {
					defer exchange.Close()
				}
				wantCalls := websocketQuotaOwnerA
				if route == quotamodel.RouteKindCompact {
					wantCalls += "," + websocketQuotaOwnerB
					if err != nil || exchange == nil || exchange.Result.AccountID != websocketQuotaOwnerB || exchange.Result.Response.StatusCode != 200 {
						t.Fatalf("unowned previous ID blocked Compact recovery: exchange=%v err=%v", exchange, err)
					}
				} else {
					var failure *proxymodel.Error
					if !errors.As(err, &failure) || failure.StatusCode != http.StatusServiceUnavailable || failure.Message != upstreamTransportFailureMessage {
						t.Fatalf("unowned previous ID authorized an HTTP turn reset: %v", err)
					}
				}
				if strings.Join(gateway.calls, ",") != wantCalls {
					t.Fatalf("mixed identity dispatches=%v want=%s", gateway.calls, wantCalls)
				}
				for _, sent := range gateway.requests {
					if string(sent.Body) != body || sent.Header.Get("x-codex-turn-state") != "turn-mixed" {
						t.Fatal("mixed identity recovery changed the opaque request")
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
				turnOwner := websocketQuotaOwnerA
				if route == quotamodel.RouteKindCompact {
					turnOwner = ""
				}
				for _, want := range []struct {
					keys  affinityKeys
					owner string
				}{{affinityKeys{turn: "turn-mixed"}, turnOwner}, {affinityKeys{previous: "resp-unrelated"}, websocketQuotaOwnerA}, {affinityKeys{previous: "resp-unseen"}, ""}} {
					owner, err := store.owner(t.Context(), want.keys, router.now())
					if err != nil || owner != want.owner {
						t.Fatalf("mixed identity ownership=%q err=%v want=%q", owner, err, want.owner)
					}
				}
				owner, err := store.owner(t.Context(), affinityKeys{session: "session-mixed"}, router.now())
				if route == quotamodel.RouteKindCompact {
					if owner != "" || err == nil || err.Error() != "request contains conflicting account affinity" {
						t.Fatalf("Compact session lost its conflict: owner=%q err=%v", owner, err)
					}
				} else if err != nil || owner != websocketQuotaOwnerA {
					t.Fatalf("HTTP turn transport lost its session: owner=%q err=%v", owner, err)
				}
			})
		}
	}
}
