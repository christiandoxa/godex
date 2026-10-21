package jsonutil

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// Canonical serde_json 1.0.151 outputs were verified with the pinned Rust
// crate. Unlike encoding/json they retain HTML and Unicode line separators,
// and normalize numerical spelling using serde's default f64 parser.
func TestProdex04384SerdeValueWriterCanonicalOutput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input any
		want  string
	}{
		{"null", nil, `null`},
		{"bools", []any{true, false, nil}, `[true,false,null]`},
		{"uint64-max", json.Number("18446744073709551615"), `18446744073709551615`},
		{"high-unsigned-overflow", json.Number("18446744073709551616"), `1.8446744073709552e+19`},
		{"negative-zero", json.Number("-0"), `-0.0`},
		{"float-one", json.Number("1.000000000000000000"), `1.0`},
		{"decimal-lower-fixed", json.Number("1e-5"), `0.00001`},
		{"decimal-lower-scientific", json.Number("1e-6"), `1e-6`},
		{"decimal-upper-fixed", json.Number("1e15"), `1000000000000000.0`},
		{"decimal-upper-scientific", json.Number("1e16"), `1e+16`},
		{"serde-near-max-finite", json.Number("1.79769313486231580e308"), `1.7976931348623157e+308`},
		{"HTML", map[string]any{"a": "<tag>&=/"}, `{"a":"<tag>&=/"}`},
		{"Unicode-line-separators", map[string]any{"a": "\u2028\u2029"}, "{\"a\":\"\u2028\u2029\"}"},
		{"escaped-slash-and-literal", map[string]any{"a": `\u2028`}, `{"a":"\\u2028"}`},
		{"control-characters", map[string]any{"a": "\b\t\n\f\r\x00\x1f"}, `{"a":"\b\t\n\f\r\u0000\u001f"}`},
		{"sorted-recursive", map[string]any{"z": []any{map[string]any{"y": json.Number("1e-7"), "x": "<"}}, "a": true}, `{"a":true,"z":[{"x":"<","y":1e-7}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := MarshalSerdeValue04384(tc.input)
			if err != nil || string(data) != tc.want {
				t.Fatalf("canonical value=%#v -> %q error=%v want=%q", tc.input, data, err, tc.want)
			}
		})
	}
}

func TestProdex04384SerdeValueWriterRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input any
	}{
		{"invalid-surrogate-byte", string([]byte{0xff})},
		{"nan", math.NaN()},
		{"infinity", math.Inf(1)},
		{"numeric-overflow", json.Number("1.7976931348623158e308")},
		{"invalid-number", json.Number("01")},
		{"unsupported", make(chan int)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if data, err := MarshalSerdeValue04384(tc.input); err == nil {
				t.Fatalf("invalid serde value produced %q", data)
			}
		})
	}
	deep := any(0)
	for i := 0; i < 131; i++ {
		deep = []any{deep}
	}
	if result, err := MarshalSerdeValue04384(deep); err == nil {
		t.Fatalf("excessive nesting serialized: %d bytes", len(result))
	}
	if got, err := MarshalSerdeValue04384(map[string]any{"safe": strings.Repeat("x", 1024)}); err != nil || len(got) != 1035 {
		t.Fatalf("ordinary metadata failed=%v len=%d", err, len(got))
	}
}
