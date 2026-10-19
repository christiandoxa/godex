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
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04383FullHistoryWaitsForBusyAlternateUntilCancellation(t *testing.T) {
	type requestContextKey struct{}
	for _, failure := range []string{"transport", "quota"} {
		for _, persisted := range []bool{false, true} {
			t.Run(failure+map[bool]string{false: "/memory", true: "/durable"}[persisted], func(t *testing.T) {
				response := &proxymodel.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")), WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true}}
				if failure == "quota" {
					response = websocketQuotaFailureResponse()
				}
				gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{response, websocketCommittedTestResponse("unexpected-replay")}}
				ctx, cancel := context.WithCancel(context.WithValue(t.Context(), requestContextKey{}, "original-request"))
				defer cancel()
				waits := 0
				now := time.Now()
				config := Config{
					Gateway: gateway, PreferredAccount: websocketQuotaOwnerA, ProfileInflightHardLimit: 2,
					Now:      func() time.Time { return now },
					Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil },
					ProfileInflightWait: func(waitCtx context.Context, _ <-chan struct{}, _ time.Duration) error {
						waits++
						if waitCtx.Value(requestContextKey{}) != "original-request" {
							t.Fatal("capacity recovery lost the original request context")
						}
						if waits == 1 {
							// Cooldown expiry cannot reintroduce the failed owner into this handoff.
							now = now.Add(time.Minute)
							return nil
						}
						cancel()
						return waitCtx.Err()
					},
				}
				if persisted {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				keys := affinityKeys{turn: "turn-busy", session: "session-busy", previous: "resp-unrelated"}
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, keys, router.now()); err != nil {
					t.Fatal(err)
				}
				request := websocketDispatchRequest(`{"type":"response.create","session_id":"session-busy","x-codex-turn-state":"turn-busy","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"next"}]}`, 843308)
				release, acquired := router.tryAcquireProfileInflight(websocketQuotaOwnerB, request, false)
				if !acquired {
					t.Fatal("could not occupy alternate capacity")
				}
				defer release()
				result, err := router.Forward(ctx, request)
				if result != nil {
					_ = result.Close()
				}
				if !errors.Is(err, context.Canceled) || waits != 2 || strings.Join(gateway.accounts, ",") != websocketQuotaOwnerA {
					t.Fatalf("busy recovery did not wait without dispatch: err=%v waits=%d attempts=%v", err, waits, gateway.accounts)
				}
				store := router.affinity
				if persisted {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for _, check := range []struct {
					keys affinityKeys
					want string
				}{
					{keys: affinityKeys{turn: keys.turn}},
					{keys: affinityKeys{session: keys.session}, want: websocketQuotaOwnerA},
					{keys: affinityKeys{previous: keys.previous}, want: websocketQuotaOwnerA},
				} {
					owner, err := store.owner(t.Context(), check.keys, router.now())
					if err != nil || owner != check.want {
						t.Fatalf("capacity recovery ownership: keys=%+v owner=%q want=%q err=%v", check.keys, owner, check.want, err)
					}
				}
			})
		}
	}
}
