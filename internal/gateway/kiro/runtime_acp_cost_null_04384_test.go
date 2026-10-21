package kiro

import (
	"fmt"
	"testing"
)

// Prodex 0.438.4 implements usage cost as
//
//	value.get("cost").cloned().map(serde_json::from_value::<Cost>).transpose()
//
// Thus ABSENT cost is None, but PRESENT null is Some(Null) and fails to
// deserialize a required `Cost { amount: f64, currency: String }` struct.
// In contrast, an absent cost is a valid update and its OUTPUT includes
// cost:null. These two cases must not be conflated.
func TestProdex04384KiroACPExplicitNullCostCannotReplaceUsage(t *testing.T) {
	valid := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":12,"size":40,"cost":{"amount":0.25,"currency":"USD"}}}`)
	for _, tc := range []struct {
		name, params string
		wantUsed     uint64
		wantCost     any
	}{
		{"absent-cost-valid", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":3,"size":40}}`, 3, nil},
		{"valid-present-cost", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":3,"size":40,"cost":{"amount":0.5,"currency":"USD"}}}`, 3, 0.5},
		{"explicit-null-invalid", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":0,"size":0,"cost":null}}`, 12, 0.25},
		{"incorrect-empty-object", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":0,"size":0,"cost":{}}}`, 12, 0.25},
		{"incorrect-numeric-cost", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":0,"size":0,"cost":0}}`, 12, 0.25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			turn := collectKiroTurnState([]acpEnvelope{valid, kiroUpdateEnvelope04384(tc.params)})
			if turn.usage == nil || turn.usage["used"] != tc.wantUsed || turn.usage["size"] != uint64(40) {
				t.Fatalf("cost distinction %s: overwritten earlier valid usage: %#v", tc.name, turn.usage)
			}
			cost, present := turn.usage["cost"]
			if !present {
				t.Fatalf("cost key lost for %s: %#v", tc.name, turn.usage)
			}
			if tc.wantCost == nil {
				if cost != nil {
					t.Fatalf("omitted-cost output should have explicit null, got %#v", cost)
				}
				return
			}
			object, ok := cost.(map[string]any)
			if !ok || fmt.Sprint(object["amount"]) != fmt.Sprint(tc.wantCost) || object["currency"] != "USD" {
				t.Fatalf("cost mismatch for %s got=%#v want amount %v", tc.name, cost, tc.wantCost)
			}
		})
	}
}

func TestProdex04384KiroACPNullCostIgnoredInLiveTurn(t *testing.T) {
	state := newKiroLiveState(10, "kiro-model", "synthetic")
	valid := kiroUpdateEnvelope04384(`{"sessionId":"s","update":{"sessionUpdate":"usage_update","used":15,"size":50}}`)
	invalid := kiroUpdateEnvelope04384(`{"sessionId":"s","update":{"sessionUpdate":"usage_update","used":0,"size":1,"cost":null}}`)
	route := runtimeRoute{kind: routeChat}
	state.observe(route, valid)
	state.observe(route, invalid)
	if state.turn.usage == nil || state.turn.usage["used"] != uint64(15) || state.turn.usage["size"] != uint64(50) {
		t.Fatalf("explicit null cost fabricated live usage: %#v", state.turn.usage)
	}
}
