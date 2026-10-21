package jsonutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MarshalSerdeValue04384 writes the canonical serde_json::Value representation
// used by tagged Prodex 0.438.4's Mojo ParsedJson raw_view projector.
// encoding/json.Marshal is not interchangeable: Go escapes HTML and U+2028/
// U+2029, and preserves raw json.Number spellings instead of canonicalizing
// serde's default f64 parser. Inputs are already decoded JSON values.
func MarshalSerdeValue04384(value any) ([]byte, error) {
	var output bytes.Buffer
	if err := writeSerdeJSONValue04384(&output, value, 0); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func writeSerdeJSONValue04384(out *bytes.Buffer, value any, depth int) error {
	if depth > 128 {
		return errors.New("serde JSON recursion limit exceeded")
	}
	switch typed := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if typed {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		return writeSerdeJSONString04384(out, typed)
	case json.Number:
		return writeSerdeJSONNumber04384(out, typed)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return errors.New("non-finite serde JSON number")
		}
		out.WriteString(FormatSerdeDefaultFloat(typed))
	case int:
		out.WriteString(strconv.Itoa(typed))
	case int64:
		out.WriteString(strconv.FormatInt(typed, 10))
	case uint64:
		out.WriteString(strconv.FormatUint(typed, 10))
	case []any:
		out.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := writeSerdeJSONValue04384(out, item, depth+1); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		out.WriteByte('{')
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for index, key := range keys {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := writeSerdeJSONString04384(out, key); err != nil {
				return err
			}
			out.WriteByte(':')
			if err := writeSerdeJSONValue04384(out, typed[key], depth+1); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("unsupported serde JSON value %T", value)
	}
	return nil
}

func writeSerdeJSONString04384(out *bytes.Buffer, value string) error {
	if !utf8.ValidString(value) {
		return errors.New("invalid serde JSON UTF-8 string")
	}
	out.WriteByte('"')
	const hex = "0123456789abcdef"
	for _, char := range value {
		switch char {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if char < 0x20 {
				out.WriteString(`\u00`)
				out.WriteByte(hex[(char>>4)&15])
				out.WriteByte(hex[char&15])
			} else {
				// serde emits ordinary valid UTF-8 (including U+2028/2029)
				// literally, not Go's optional JS/HTML escapes.
				out.WriteRune(char)
			}
		}
	}
	out.WriteByte('"')
	return nil
}

func writeSerdeJSONNumber04384(out *bytes.Buffer, value json.Number) error {
	original := string(value)
	if !json.Valid([]byte(original)) {
		return errors.New("invalid serde JSON number")
	}
	if !strings.ContainsAny(original, ".eE") && original != "-0" {
		if original[0] == '-' {
			if _, err := strconv.ParseInt(original, 10, 64); err == nil {
				out.WriteString(original)
				return nil
			}
		} else if _, err := strconv.ParseUint(original, 10, 64); err == nil {
			out.WriteString(original)
			return nil
		}
	}
	parsed, valid := ParseSerdeDefaultFloat(original)
	if !valid {
		return errors.New("serde JSON number out of range")
	}
	out.WriteString(FormatSerdeDefaultFloat(parsed))
	return nil
}
