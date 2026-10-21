package routing

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Prodex serializes the parsed serde_json::Value before passing its full
// document to the Mojo quota_reset_json_epoch_v1 rich-view validator, which
// caps the CANONICAL payload at 16MiB. Do not cap raw request length:
// whitespace discarded by serde serialization can make a larger raw input
// valid. Disable Go's HTML escaping to approximate Rust serde's canonical
// JSON and count the output bytes without retaining a second large buffer.
const quotaCanonicalJSONLimit04383 = 16 << 20

func quotaCanonicalJSONWithinLimit04383(value any) bool {
	size, valid := quotaCanonicalJSONSize04383(value)
	return valid && size <= quotaCanonicalJSONLimit04383
}

// The Mojo rich-view cap applies to serde_json::to_string(&Value), not to
// Go's raw json.Number spellings nor the HTML/line-separator escaping that
// encoding/json adds. Compute the size of the canonical tree without
// allocating another potentially huge serialized payload.
func quotaCanonicalJSONSize04383(value any) (int, bool) {
	switch typed := value.(type) {
	case nil:
		return 4, true
	case bool:
		if typed {
			return 4, true
		}
		return 5, true
	case string:
		return quotaCanonicalStringSize04383(typed), true
	case json.Number:
		return quotaCanonicalNumberSize04383(typed)
	case []any:
		size := 2
		for index, element := range typed {
			member, valid := quotaCanonicalJSONSize04383(element)
			if !valid {
				return 0, false
			}
			if index > 0 {
				size++
			}
			size += member
			if size > quotaCanonicalJSONLimit04383 {
				return size, true
			}
		}
		return size, true
	case map[string]any:
		size := 2
		index := 0
		for key, element := range typed {
			member, valid := quotaCanonicalJSONSize04383(element)
			if !valid {
				return 0, false
			}
			if index > 0 {
				size++
			}
			// One ':' separator after the canonical quoted key.
			size += quotaCanonicalStringSize04383(key) + 1 + member
			if size > quotaCanonicalJSONLimit04383 {
				return size, true
			}
			index++
		}
		return size, true
	default:
		return 0, false
	}
}

func quotaCanonicalStringSize04383(value string) int {
	size := 2 // surrounding JSON quotes
	for at := 0; at < len(value); at++ {
		switch value[at] {
		case '\b', '\f', '\n', '\r', '\t', '"', '\\':
			size += 2
		default:
			if value[at] < 0x20 {
				size += 6 // serde_json \u00XX for other control bytes
			} else {
				// serde_json leaves valid UTF-8, including U+2028/U+2029,
				// raw; Go's JSON encoder would escape those code points.
				size++
			}
		}
	}
	return size
}

func quotaCanonicalNumberSize04383(number json.Number) (int, bool) {
	text := string(number)
	if !strings.ContainsAny(text, ".eE") && text != "-0" {
		if text[0] == '-' {
			if _, err := strconv.ParseInt(text, 10, 64); err == nil {
				return len(text), true
			}
		} else if _, err := strconv.ParseUint(text, 10, 64); err == nil {
			return len(text), true
		}
	}
	// serde_json converts fractional/exponential/out-of-range numeric
	// literals to f64, then formats the *numeric value*, not its original
	// digits. This can shrink a 17MiB `1.000...` into the 3 bytes `1.0`.
	floating, valid := quotaSerdeFloat04383(text)
	if !valid {
		return 0, false
	}
	// serde_json's ryu formatter uses fixed decimal notation for
	// magnitudes in [1e-5, 1e16), scientific otherwise. Go's 'g'
	// switches to scientific notation at much smaller magnitudes and
	// zero-pads small exponents (1e-07 versus serde's 1e-7).
	magnitude := floating
	if magnitude < 0 {
		magnitude = -magnitude
	}
	var formatted string
	if magnitude == 0 || magnitude >= 1e-5 && magnitude < 1e16 {
		formatted = strconv.FormatFloat(floating, 'f', -1, 64)
		if !strings.Contains(formatted, ".") {
			formatted += ".0"
		}
	} else {
		formatted = strconv.FormatFloat(floating, 'e', -1, 64)
		if index := strings.LastIndexByte(formatted, 'e'); index >= 0 && index+2 < len(formatted) {
			prefix, exponent := formatted[:index+2], formatted[index+2:]
			exponent = strings.TrimLeft(exponent, "0")
			if exponent == "" {
				exponent = "0"
			}
			formatted = prefix + exponent
		}
	}
	return len(formatted), true
}
