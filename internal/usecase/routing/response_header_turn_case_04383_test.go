package routing

import (
	"testing"
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
