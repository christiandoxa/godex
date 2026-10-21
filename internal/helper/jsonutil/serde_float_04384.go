package jsonutil

import (
	"math"
	"strconv"
)

// The published 0.438.4 lockfile's serde_json 1.0.154 has float_roundtrip disabled.
// Its deserializer retains a u64 significand until integer/decimal overflow,
// then applies a decimal exponent using compile-time nearest-f64 POW10
// constants. Go's strconv.ParseFloat uses a different correctly rounded
// decimal algorithm; math.Pow10 also does not always yield Rust's literal
// 1eN constant. Either difference changes the canonical string length
// and can cross the Mojo quota metadata 16MiB admission boundary.
// strconv.ParseFloat("1eN") gives the same correctly rounded f64 as the
// published serde_json/de.rs POW10 literal for every exponent 0..308.
var serdePow10_04384 = func() [309]float64 {
	var powers [309]float64
	for i := range powers {
		powers[i], _ = strconv.ParseFloat("1e"+strconv.Itoa(i), 64)
	}
	return powers
}()

// ParseSerdeDefaultFloat is called on an already whole-document validated
// serde-compatible JSON number. It intentionally reproduces the simple
// significand/exponent parser in serde_json 1.0.154/src/de.rs rather than
// reparsing the original decimal with Go's strconv.ParseFloat.
func ParseSerdeDefaultFloat(text string) (float64, bool) {
	if text == "" {
		return 0, false
	}
	positive := true
	if text[0] == '-' {
		positive = false
		text = text[1:]
	}

	var significand uint64
	var exponentBefore, exponentAfter int32
	var explicitExponent int32
	var exponentSign int32 = 1
	part := 0 // integer, fraction, exponent
	significandOverflow := false
	exponentOverflow := false

	for i := 0; i < len(text); i++ {
		value := text[i]
		switch value {
		case '.':
			part = 1
			continue
		case 'e', 'E':
			part = 2
			continue
		case '-', '+':
			if part == 2 {
				if value == '-' {
					exponentSign = -1
				}
				continue
			}
		}
		digit := uint64(value - '0')
		if part == 2 {
			if !exponentOverflow {
				if explicitExponent > (math.MaxInt32-int32(digit))/10 {
					exponentOverflow = true
				} else {
					explicitExponent = explicitExponent*10 + int32(digit)
				}
			}
			continue
		}
		if significandOverflow {
			if part == 0 {
				exponentBefore++
			}
			continue
		}
		if significand > (math.MaxUint64-digit)/10 {
			significandOverflow = true
			if part == 0 {
				exponentBefore++
			}
			continue
		}
		significand = significand*10 + digit
		if part == 1 {
			exponentAfter--
		}
	}

	if exponentOverflow {
		if exponentSign < 0 || significand == 0 {
			if positive {
				return 0, true
			}
			return math.Copysign(0, -1), true
		}
		return 0, false
	}
	exponent := exponentBefore + exponentAfter + explicitExponent*exponentSign
	floating := float64(significand)
	for {
		if exponent >= -308 && exponent <= 308 {
			if exponent >= 0 {
				floating *= serdePow10_04384[exponent]
				if math.IsInf(floating, 0) {
					return 0, false
				}
			} else {
				floating /= serdePow10_04384[-exponent]
			}
			break
		}
		if floating == 0 {
			break
		}
		if exponent > 308 {
			return 0, false
		}
		floating /= 1e308
		exponent += 308
	}
	if !positive {
		floating = -floating
	}
	return floating, true
}
