package routing

import (
	"context"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// The tagged compiled runtime_response_metadata_json checks the direct
// response.id, top-level response_id, or qualifying top-level id ONLY.
// Arbitrary data/result/payload wrappers cannot introduce hard response or
// turn IDs, even if their JSON fields look like response metadata.
func TestProdex04383NestedOpaqueResponsePayloadCannotMintAffinity(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, payload          string
			responseID, turnID     string
			wantPrevious, wantTurn bool
		}{
			{"data-id", `{"type":"event","data":{"id":"untrusted-response"}}`, "untrusted-response", "", false, false},
			{"result-id", `{"type":"event","result":{"id":"untrusted-response"}}`, "untrusted-response", "", false, false},
			{"payload-id", `{"type":"event","payload":{"id":"untrusted-response"}}`, "untrusted-response", "", false, false},
			{"data-response-id", `{"type":"event","data":{"response_id":"untrusted-response"}}`, "untrusted-response", "", false, false},
			{"data-turn", `{"object":"event","data":{"turn_state":"untrusted-turn"}}`, "", "untrusted-turn", false, false},
			{"result-turn", `{"object":"event","result":{"turn_state":"untrusted-turn"}}`, "", "untrusted-turn", false, false},
			{"payload-turn", `{"object":"event","payload":{"turn_state":"untrusted-turn"}}`, "", "untrusted-turn", false, false},
			{"canonical-nested-response", `{"type":"event","response":{"id":"untrusted-response","turn_state":"untrusted-turn"}}`, "untrusted-response", "untrusted-turn", true, true},
			{"canonical-root", `{"object":"response","id":"untrusted-response","turn_state":"untrusted-turn"}`, "untrusted-response", "untrusted-turn", true, true},
			{"canonical-legacy", `{"type":"event","response_id":"untrusted-response"}`, "untrusted-response", "", true, false},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				config := Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				if err := router.Observe(t.Context(), websocketQuotaOwnerA, make(http.Header), []byte(tc.payload), false); err != nil {
					t.Fatal(err)
				}
				keys := responseAffinity(nil, []byte(tc.payload), false)
				if tc.responseID != "" {
					store := router.affinity
					if durable {
						store = newAffinityStore()
						store.repository = config.Bindings
					}
					owner, err := store.owner(t.Context(), affinityKeys{previous: tc.responseID}, router.now())
					if err != nil {
						t.Fatal(err)
					}
					want := ""
					if tc.wantPrevious {
						want = websocketQuotaOwnerA
					}
					if owner != want {
						t.Fatalf("opaque %s response owner=%q want=%q parsed=%+v", tc.name, owner, want, keys)
					}
				}
				if tc.turnID != "" {
					store := router.affinity
					if durable {
						store = newAffinityStore()
						store.repository = config.Bindings
					}
					owner, err := store.owner(t.Context(), affinityKeys{turn: tc.turnID}, router.now())
					if err != nil {
						t.Fatal(err)
					}
					want := ""
					if tc.wantTurn {
						want = websocketQuotaOwnerA
					}
					if owner != want {
						t.Fatalf("opaque %s turn owner=%q want=%q parsed=%+v", tc.name, owner, want, keys)
					}
				}
			})
		}
	}
}

// The same metadata rule applies to each committed SSE event. A nested
// data/result/payload object is part of the event payload, not an implicit
// response envelope carrying an account-binding identity.
func TestProdex04383SSEOpaqueWrappersCannotBindContinuation(t *testing.T) {
	for _, durable := range []bool{false, true} {
		config := Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
		if durable {
			config.Bindings = routingrepository.NewStore(t.TempDir())
		}
		router, err := NewRouter(config)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			id, payload string
			want        bool
		}{
			{"untrusted-data", `{"type":"response.completed","data":{"id":"untrusted-data"}}`, false},
			{"untrusted-payload", `{"type":"response.completed","payload":{"id":"untrusted-payload"}}`, false},
			{"untrusted-result", `{"type":"response.completed","result":{"id":"untrusted-result"}}`, false},
			{"canonical-response", `{"type":"response.completed","response":{"id":"canonical-response"}}`, true},
		} {
			wire := []byte("data: " + tc.payload + "\n\n")
			if err := router.Observe(t.Context(), websocketQuotaOwnerA, make(http.Header), wire, true); err != nil {
				t.Fatal(err)
			}
			store := router.affinity
			if durable {
				store = newAffinityStore()
				store.repository = config.Bindings
			}
			owner, err := store.owner(t.Context(), affinityKeys{previous: tc.id}, router.now())
			want := ""
			if tc.want {
				want = websocketQuotaOwnerA
			}
			if err != nil || owner != want {
				t.Fatalf("SSE %s owner=%q want=%q err=%v", tc.id, owner, want, err)
			}
		}
		router.Close()
	}
}
