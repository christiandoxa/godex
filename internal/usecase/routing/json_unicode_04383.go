package routing

import (
	"bytes"
	"unicode/utf8"
)

// serde_json rejects invalid UTF-8 and unpaired UTF-16 surrogate escapes.
// encoding/json silently replaces both with U+FFFD. Keep the original bytes
// for forwarding, but never derive affinity or usage from invalid JSON text.
// The normal JSON decoder still owns all other syntax and schema validation.
func validNativeJSONUnicode04383(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	if !bytes.Contains(data, []byte(`\u`)) {
		return true
	}
	inString := false
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || i+1 >= len(data) {
				continue
			}
			if data[i+1] != 'u' {
				i++ // Escaped quote or slash cannot begin a unicode escape.
				continue
			}
			value, valid := nativeJSONHex4(data, i+2)
			if !valid {
				return false
			}
			i += 5 // Last hex digit of the first unicode escape.
			switch {
			case value >= 0xdc00 && value <= 0xdfff:
				return false
			case value >= 0xd800 && value <= 0xdbff:
				if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
					return false
				}
				low, valid := nativeJSONHex4(data, i+3)
				if !valid || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6 // Skip the validated low surrogate escape.
			}
		}
	}
	return true
}

func nativeJSONHex4(data []byte, start int) (uint16, bool) {
	if start < 0 || start+4 > len(data) {
		return 0, false
	}
	var value uint16
	for _, digit := range data[start : start+4] {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value |= uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value |= uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
