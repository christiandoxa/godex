package jsonutil

import "testing"

// These serde_json 1.0.151 whole-document outcomes were checked with the
// exact pinned Rust crate. The Go default JSON decoder alone is insufficient
// for the near-max-f64 boundary and nested values that are later ignored.
func TestProdex04384SerdeNumberTokensPreserveRustValidity(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		valid     bool
	}{
		{"ordinary", `{"n":[0,-1,18446744073709551615]}`, true},
		{"valid-f64-max", `{"n":1.7976931348623157e308}`, true},
		{"invalid-f64-near-max", `{"n":1.7976931348623158e308}`, false},
		{"long-mantissa-rounds-in-range", `{"n":1.79769313486231580e308}`, true},
		{"nested-invalid", `{"ignored":{"inner":[1e309]}}`, false},
		{"valid-high-exponent-zero", `{"n":0e99999999999999999999}`, true},
		{"malformed", `{"n":1e`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ok := CheckSerdeNumberTokens([]byte(tc.raw)); ok != tc.valid {
				t.Fatalf("serde numeric JSON %q accepted=%t want=%t", tc.raw, ok, tc.valid)
			}
		})
	}
}
