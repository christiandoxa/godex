package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// The tagged Mojo runtime_response_metadata_json_v1 classifies a top-level
// object.id using the untrimmed JSON `object` string: exactly "response" or
// any value whose raw bytes end in ".response". Surrounding whitespace
// cannot be normalized into a response ID and mint a hard owner.
func TestProdex04383ResponseObjectKindExactSuffixNoTrimming(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		want       bool
	}{
		{"canonical", "response", true},
		{"canonical-prefix", "response.v1.response", true},
		{"leading-other-prefix", " response.v1.response", true},
		{"leading-space-canonical", " response", false},
		{"trailing-space-canonical", "response ", false},
		{"both-space-canonical", " response ", false},
		{"trailing-space-suffix", "response.v1.response ", false},
		{"trailing-tab-suffix", "response.v1.response\t", false},
		{"blank-type", " \t ", false},
		{"unrelated", "event", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"object":` + quotedObjectKind04383(tc.kind) + `,"id":"untrusted-root-id","output":[]}`)
			keys, ids := responseAffinityWithIDs(make(http.Header), body, false)
			hasID := len(ids) == 1 && ids[0] == "untrusted-root-id"
			if hasID != tc.want || (tc.want && keys.previous != "untrusted-root-id") || (!tc.want && keys.previous != "") {
				t.Fatalf("response object kind=%q incorrectly admitted ID: ids=%v keys=%+v want=%t", tc.kind, ids, keys, tc.want)
			}
		})
	}
}

func TestProdex04383ResponseKindWhitespaceCannotMintDurableOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, kind string
			bound      bool
		}{
			{"canonical", "response", true},
			{"canonical-suffix", "v1.response", true},
			{"leading-space", " response", false},
			{"trailing-space", "response ", false},
			{"padded-suffix", "v1.response ", false},
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
				body := []byte(`{"object":` + quotedObjectKind04383(tc.kind) + `,"id":"object-only-root","output":[]}`)
				if err := router.Observe(t.Context(), websocketQuotaOwnerA, make(http.Header), body, false, "openai"); err != nil {
					t.Fatal(err)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), affinityKeys{previous: "object-only-root"}, router.now())
				if err != nil {
					t.Fatal(err)
				}
				want := ""
				if tc.bound {
					want = websocketQuotaOwnerA
				}
				if owner != want {
					t.Fatalf("object kind %q minting unexpected owner=%q want=%q", tc.kind, owner, want)
				}
			})
		}
	}
}

func quotedObjectKind04383(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
