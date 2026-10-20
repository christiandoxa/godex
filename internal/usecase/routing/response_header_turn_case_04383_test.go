package routing

import (
	"context"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// Tagged 0.438.3 sorts serde_json object keys before its compiled metadata
// scanner sees them. Header names are ASCII-folded without trimming or Unicode
// simple folding; an array of headers retains its original entry order.
func TestProdex04383ResponseEmbeddedTurnHeaderExactASCIIAndOrder(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"sorted-case-keys", `{"object":"response","id":"resp","headers":{"x-codex-turn-state":"lower","X-CODEX-TURN-STATE":"upper"}}`, "upper"},
		{"sorted-case-three", `{"object":"response","id":"resp","headers":{"x-codex-turn-state":"lower","X-CODEX-TURN-STATE":"upper","X-Codex-Turn-State":"middle"}}`, "upper"},
		{"reject-unicode-casefold", `{"object":"response","id":"resp","headers":{"x-codex-turn-ſtate":"impostor"}}`, ""},
		{"reject-whitespace-key", `{"object":"response","id":"resp","headers":{" x-codex-turn-state ":"impostor"}}`, ""},
		{"ignore-impostor-select-real", `{"object":"response","id":"resp","headers":{"x-codex-turn-ſtate":"impostor","x-codex-turn-state":"legitimate"}}`, "legitimate"},
		{"array-order", `{"object":"response","id":"resp","headers":[["x-codex-turn-state","lower"],["X-CODEX-TURN-STATE","upper"]]}`, "lower"},
		{"object-array-fallback", `{"object":"response","id":"resp","headers":[{"name":"x-codex-turn-ſtate","value":"impostor"},{"key":"X-CODEX-TURN-STATE","values":["  ","real"]}]}`, "real"},
		// JSON null is a PRESENT member. It shadows fallback name/key or
		// value/values, matching serde_json/Mojo member lookup precedence.
		{"null-name-shadows-key", `{"object":"response","id":"resp","headers":[{"name":null,"key":"x-codex-turn-state","value":"foreign"}]}`, ""},
		{"null-value-shadows-values", `{"object":"response","id":"resp","headers":[{"name":"x-codex-turn-state","value":null,"values":["foreign"]}]}`, ""},
		{"null-name-followed-by-real", `{"object":"response","id":"resp","headers":[{"name":null,"key":"x-codex-turn-state","value":"foreign"},{"name":"x-codex-turn-state","value":"real"}]}`, "real"},
		{"null-value-followed-by-real", `{"object":"response","id":"resp","headers":[{"name":"x-codex-turn-state","value":null,"values":["foreign"]},{"key":"x-codex-turn-state","values":["real"]}]}`, "real"},
		{"missing-name-falls-back", `{"object":"response","id":"resp","headers":[{"key":"x-codex-turn-state","value":"real"}]}`, "real"},
		{"missing-value-falls-back", `{"object":"response","id":"resp","headers":[{"name":"x-codex-turn-state","values":["real"]}]}`, "real"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for range 64 {
				got := responseAffinity(nil, []byte(tc.body), false).turn
				if got != tc.want {
					t.Fatalf("embedded response turn state=%q want=%q", got, tc.want)
				}
			}
		})
	}
}

// A malformed header-array entry must not mint a persistent turn owner on
// an unrelated account. The response ID itself remains a valid observation.
func TestProdex04383HeaderArrayNullMemberCannotPersistTurnOwner(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, headers, expectedTurn string
		}{
			{"name-null", `[{"name":null,"key":"x-codex-turn-state","value":"foreign-turn"}]`, ""},
			{"value-null", `[{"name":"x-codex-turn-state","value":null,"values":["foreign-turn"]}]`, ""},
			{"name-null-next-valid", `[{"name":null,"key":"x-codex-turn-state","value":"foreign-turn"},{"name":"x-codex-turn-state","value":"real-turn"}]`, "real-turn"},
			{"value-null-next-valid", `[{"name":"x-codex-turn-state","value":null,"values":["foreign-turn"]},{"key":"x-codex-turn-state","values":["real-turn"]}]`, "real-turn"},
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
				payload := []byte(`{"object":"response","id":"resp-header-array","headers":` + tc.headers + `}`)
				if err := router.Observe(t.Context(), websocketQuotaOwnerA, make(http.Header), payload, false); err != nil {
					t.Fatal(err)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				for _, check := range []struct{ kind, value, want string }{
					{"previous", "resp-header-array", websocketQuotaOwnerA},
					{"turn", "foreign-turn", ""},
					{"turn", "real-turn", func() string {
						if tc.expectedTurn == "real-turn" {
							return websocketQuotaOwnerA
						}
						return ""
					}()},
				} {
					keys := affinityKeys{}
					if check.kind == "previous" {
						keys.previous = check.value
					} else {
						keys.turn = check.value
					}
					owner, err := store.owner(t.Context(), keys, router.now())
					if err != nil || owner != check.want {
						t.Fatalf("%s=%s owner=%q want=%q err=%v", check.kind, check.value, owner, check.want, err)
					}
				}
			})
		}
	}
}
