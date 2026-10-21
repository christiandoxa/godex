package kiro

import (
	"encoding/json"
	"testing"
)

// RuntimeKiroAcpCost.amount is a typed f64 in tagged Rust. Even when the
// original ACP JSON uses an integer, a long decimal or -0, serde serializes
// a canonical f64 when forwarding the usage metadata to the Mojo kernel.
// Go json.Number previously preserved the raw lexical representation.
func TestProdex04384KiroACPUsageCostCanonicalF64MatchesSerde(t *testing.T) {
	for _, tc := range []struct{ name, literal, expected string }{
		{"decimal-trailing-zeros", "1.00000000000", `{"amount":1.0,"currency":"USD"}`},
		{"integer-as-float", "5", `{"amount":5.0,"currency":"USD"}`},
		{"negative-zero", "-0", `{"amount":-0.0,"currency":"USD"}`},
		{"small-exponent", "1e-5", `{"amount":0.00001,"currency":"USD"}`},
		{"large-exponent", "1e16", `{"amount":1e+16,"currency":"USD"}`},
		{"large-integer", "18446744073709551615", `{"amount":1.8446744073709552e+19,"currency":"USD"}`},
		{"precise-fraction", "3.14159265358979323846", `{"amount":3.141592653589793,"currency":"USD"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := `{"sessionId":"s","update":{"sessionUpdate":"usage_update","used":12,"size":40,"cost":{"amount":` + tc.literal + `,"currency":"USD"}}}`
			turn := collectKiroTurnState([]acpEnvelope{kiroUpdateEnvelope04384(params)})
			cost := turn.usage["cost"]
			encoded, err := json.Marshal(cost)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != tc.expected {
				t.Fatalf("ACP cost.amount=%s serialized %s want %s", tc.literal, encoded, tc.expected)
			}
		})
	}
}

// Rust's provider core always serializes the *output* {"cost":null}
// when there is no cost. Only ABSENT cost at the ACP update input is
// accepted as None; explicitly PRESENT null is a serde Cost parse failure.
func TestProdex04384KiroACPUsageWithoutCostRetainsNullField(t *testing.T) {
	for _, tc := range []struct{ name, extra string }{
		{"missing-cost", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := `{"sessionId":"s","update":{"sessionUpdate":"usage_update","used":1,"size":9` + tc.extra + `}}`
			turn := collectKiroTurnState([]acpEnvelope{kiroUpdateEnvelope04384(params)})
			encoded, err := json.Marshal(turn.usage)
			if err != nil {
				t.Fatal(err)
			}
			const want = `{"cost":null,"remaining":8,"size":9,"used":1}`
			if string(encoded) != want {
				t.Fatalf("Kiro optional cost not preserved as null: got %s want %s", encoded, want)
			}
		})
	}
}

func TestProdex04384KiroACPUsageCostUnderflowRemainsValid(t *testing.T) {
	for _, literal := range []string{"1e-999", "0e999999999999999999999"} {
		params := `{"sessionId":"s","update":{"sessionUpdate":"usage_update","used":2,"size":5,"cost":{"amount":` + literal + `,"currency":"USD"}}}`
		turn := collectKiroTurnState([]acpEnvelope{kiroUpdateEnvelope04384(params)})
		encoded, err := json.Marshal(turn.usage["cost"])
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"amount":0.0,"currency":"USD"}`
		if string(encoded) != want {
			t.Fatalf("serde Kiro cost numeric underflow=%s got=%s want=%s", literal, encoded, want)
		}
	}
}
