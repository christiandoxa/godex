package chatcompat

import (
	"encoding/json"
	"math"
	"testing"
)

func TestProdex04384ChatResponseUnsignedPresenceAndRange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  uint64
		valid bool
	}{
		{"zero", json.Number("0"), 0, true},
		{"signed-high", json.Number("9223372036854775808"), 1 << 63, true},
		{"unsigned-max", json.Number("18446744073709551615"), ^uint64(0), true},
		{"host-unsigned-max", ^uint64(0), ^uint64(0), true},
		{"missing", nil, 0, false},
		{"negative", json.Number("-1"), 0, false},
		{"negative-zero", json.Number("-0"), 0, false},
		{"decimal", json.Number("1.0"), 0, false},
		{"exponent", json.Number("1e0"), 0, false},
		{"overflow", json.Number("18446744073709551616"), 0, false},
		{"legacy-integral-float", float64(2), 2, true},
		{"legacy-overflow-float", float64(0x1p64), 0, false},
		{"nan", math.NaN(), 0, false},
		{"infinity", math.Inf(1), 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, valid := uintValue(tc.value)
			if valid != tc.valid || valid && got != tc.want {
				t.Errorf("unsigned scalar=%d valid=%v want=%d valid=%v", got, valid, tc.want, tc.valid)
			}
			if !valid && uintValueOr(tc.value, 17) != 17 {
				t.Error("invalid value did not preserve its caller's fallback")
			}
		})
	}
}
