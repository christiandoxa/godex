package jsonutil

import (
	"errors"
	"strconv"
	"unicode/utf8"
)

// CheckSerdeBoundary enforces serde_json encoding and recursion boundaries
// before Go's JSON decoder.
// Go otherwise replaces unmatched Unicode surrogates and admits nested JSON
// beyond the 127-container recursion bound. Structural syntax and finite
// numeric range are still checked by encoding/json at the owning caller.
func CheckSerdeBoundary(raw string) error {
	if !utf8.ValidString(raw) {
		return errors.New("invalid UTF-8 in JSON")
	}
	depth, insideString := 0, false
	for pos := 0; pos < len(raw); pos++ {
		switch raw[pos] {
		case '"':
			insideString = !insideString
		case '\\':
			if !insideString {
				continue
			}
			if pos+1 >= len(raw) {
				return errors.New("unterminated JSON escape")
			}
			if raw[pos+1] != 'u' {
				pos++
				continue
			}
			unit, ok := unicodeUnit(raw, pos+2)
			if !ok {
				return errors.New("invalid JSON Unicode escape")
			}
			switch {
			case unit >= 0xD800 && unit <= 0xDBFF:
				if pos+12 > len(raw) || raw[pos+6:pos+8] != `\u` {
					return errors.New("unpaired JSON high surrogate")
				}
				next, valid := unicodeUnit(raw, pos+8)
				if !valid || next < 0xDC00 || next > 0xDFFF {
					return errors.New("unpaired JSON high surrogate")
				}
				pos += 11
			case unit >= 0xDC00 && unit <= 0xDFFF:
				return errors.New("unpaired JSON low surrogate")
			default:
				pos += 5
			}
		case '[', '{':
			if !insideString {
				depth++
				if depth > 127 {
					return errors.New("JSON nesting exceeds 127 containers")
				}
			}
		case ']', '}':
			if !insideString {
				depth--
			}
		}
	}
	return nil
}

func unicodeUnit(raw string, offset int) (uint64, bool) {
	if offset+4 > len(raw) {
		return 0, false
	}
	unit, err := strconv.ParseUint(raw[offset:offset+4], 16, 16)
	return unit, err == nil
}
