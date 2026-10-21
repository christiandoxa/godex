package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// The tagged 0.438.3 unbound WebSocket request planner applies its 12
// attempt / 3-second precommit budget when the complete JSON is rejected.
// The raw, opaque message must be sent byte-for-byte on Ready profiles;
// previous_response_not_found no longer names a trusted owner, so it may
// rotate until exhausting that finite budget. A valid tool-output frame,
// by contrast, stays with its verified previous-response owner.
func TestProdex04383InvalidSerdeWebSocketUsesBoundedFreshPrecommitBudget(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{}
			for i := 0; i < 12; i++ {
				gateway.responses = append(gateway.responses, websocketPreviousResponseFailure(""))
			}
			config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
			if durable {
				config.Bindings = routingrepository.NewStore(t.TempDir())
			}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{previous: "resp-main"}, router.now()); err != nil {
				t.Fatal(err)
			}
			body := websocketToolTypeMessageForRouting04383(false)
			req := websocketDispatchRequest(body, 843400)
			result, err := router.Forward(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if len(gateway.accounts) != 12 {
				t.Fatalf("serde-invalid bounded precommit used %d accounts %v, expected 12", len(gateway.accounts), gateway.accounts)
			}
			for i, owner := range gateway.accounts {
				want := websocketQuotaOwnerA
				if i%2 == 1 {
					want = websocketQuotaOwnerB
				}
				if owner != want {
					t.Fatalf("attempt %d owner=%q want=%q all=%v", i, owner, want, gateway.accounts)
				}
				if string(gateway.requests[i].Body) != body {
					t.Fatalf("attempt %d changed opaque frame", i)
				}
			}
			if result.Result.Response.StatusCode != http.StatusConflict {
				t.Fatalf("invalid serde precommit terminal=%d want 409", result.Result.Response.StatusCode)
			}
			output, err := io.ReadAll(result.Result.Response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(output), `"stale_continuation"`) {
				t.Fatalf("missing stale terminal: %q", output)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			owner, err := store.owner(t.Context(), affinityKeys{previous: "resp-main"}, router.now())
			if err != nil || owner != websocketQuotaOwnerA {
				t.Fatalf("invalid frame stole verified owner=%q err=%v", owner, err)
			}
		})
	}
}

func websocketToolTypeMessageForRouting04383(valid bool) string {
	literal := "1.7976931348623158e308"
	if valid {
		literal = "1.79769313486231580e308"
	}
	return `{"type":"response.create","model":"gpt-6","previous_response_id":"resp-main","input":[{"type":"function_call_output","call_id":"call-tool-suffix","output":"done"}],"unused":` + literal + `}`
}

// The unbound budget is only for repeated missing-response precommit
// failures. A truly committed fresh response or unrelated upstream
// rejection terminates after one exact request and must not be replayed.
func TestProdex04383InvalidSerdeWebSocketUnboundStopsOnUnrelatedOutcome(t *testing.T) {
	for _, tc := range []struct {
		name       string
		response   *proxymodel.Response
		wantStatus int
	}{
		{"committed", websocketCommittedTestResponse("resp-fresh"), http.StatusOK},
		{"unrelated-error", &proxymodel.Response{StatusCode: http.StatusBadRequest,
			Body:            io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_prompt"}}`)),
			WebSocketFrames: true}, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{tc.response}}
			router, err := NewRouter(Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			result, err := router.Forward(t.Context(), websocketDispatchRequest(websocketToolTypeMessageForRouting04383(false), 843401))
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if len(gateway.accounts) != 1 || result.Result.Response.StatusCode != tc.wantStatus {
				t.Fatalf("unrelated/committed serde failure replayed: accounts=%v status=%d", gateway.accounts, result.Result.Response.StatusCode)
			}
		})
	}
}
