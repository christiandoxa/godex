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

func TestProdex04383FullContextSignalRetainsAccompanyingSession(t *testing.T) {
	for _, failure := range []string{"transport", "quota", "presend-quota"} {
		for _, persisted := range []bool{false, true} {
			t.Run(failure+map[bool]string{false: "/memory", true: "/durable"}[persisted], func(t *testing.T) {
				response := websocketQuotaFailureResponse()
				if failure == "transport" {
					response = &proxymodel.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")), WebSocketFrames: true, PrecommitFailure: &proxymodel.PrecommitFailure{Transport: true}}
				}
				gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{response}}
				accounts := websocketQuotaAccounts()
				if failure == "presend-quota" {
					accounts[0].EligibleAfter = time.Now().Add(time.Hour)
				}
				config := Config{Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil }}
				if persisted {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				keys := affinityKeys{previous: "resp-original", turn: "turn-a", session: "session-a"}
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, keys, router.now()); err != nil {
					t.Fatal(err)
				}
				request := websocketDispatchRequest(`{"type":"response.create","previous_response_id":"resp-original","session_id":"session-a","client_metadata":{"x-codex-turn-state":"turn-a"},"input":[]}`, 843305)
				forwarded, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer forwarded.Close()
				assertProdex04355FullContextSignal(t, forwarded, websocketQuotaOwnerA)
				wantCalls := 1
				if failure == "presend-quota" {
					wantCalls = 0
				}
				if len(gateway.accounts) != wantCalls {
					t.Fatalf("retry signal replayed: %v", gateway.accounts)
				}
				store := router.affinity
				if persisted {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for label, key := range map[string]affinityKeys{"session": {session: keys.session}, "response": {previous: keys.previous}, "turn": {turn: keys.turn}} {
					owner, err := store.owner(t.Context(), key, router.now())
					want := ""
					if label == "session" {
						want = websocketQuotaOwnerA
					}
					if err != nil || owner != want {
						t.Fatalf("%s owner=%q err=%v, want %q", label, owner, err, want)
					}
				}
			})
		}
	}
}

func TestProdex04383HTTPFullContextSignalRetainsAccompanyingSession(t *testing.T) {
	for _, tc := range []struct {
		name string
		step prodex04356Step
	}{
		{"quota", prodex04356Step{account: "account-a", status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota","message":"usage limit"}}`}},
		{"transport", prodex04356Step{account: "account-a", err: errors.New("connection reset by peer")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &prodex04356Gateway{steps: []prodex04356Step{tc.step}}
			router := newProdex04356Router(t, gateway, prodex04356Accounts())
			keys := affinityKeys{previous: "resp-original", turn: prodex04357TurnState, session: "session-a"}
			if err := router.affinity.remember(t.Context(), "account-a", keys, router.now()); err != nil {
				t.Fatal(err)
			}
			request := prodex04357ResponsesRequest([]byte(`{"previous_response_id":"resp-original","session_id":"session-a","input":[]}`))
			forwarded, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer forwarded.Close()
			if forwarded.Result.Response.StatusCode != http.StatusBadRequest || strings.Join(gateway.calls, ",") != "account-a" {
				t.Fatalf("HTTP retry signal status=%d calls=%v", forwarded.Result.Response.StatusCode, gateway.calls)
			}
			for label, key := range map[string]affinityKeys{"session": {session: keys.session}, "response": {previous: keys.previous}, "turn": {turn: keys.turn}} {
				owner, err := router.affinity.owner(t.Context(), key, router.now())
				want := ""
				if label == "session" {
					want = "account-a"
				}
				if err != nil || owner != want {
					t.Fatalf("HTTP %s owner=%q err=%v, want %q", label, owner, err, want)
				}
			}
		})
	}
}
