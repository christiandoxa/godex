package deepseek

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func deepSeekBufferedArgumentError(name, arguments string) error {
	if reason, offset := deepSeekBufferedSerdeFailure(arguments); reason != "" {
		return deepSeekToolArgumentDiagnostic(name, arguments, reason, offset)
	}
	var value any
	cause := json.Unmarshal([]byte(arguments), &value)
	if cause == nil {
		return errors.New("DeepSeek response tool call exceeds the bounded Mojo kernel limit")
	}
	return deepSeekToolArgumentError(name, arguments, cause)
}

func deepSeekBufferedSerdeFailure(raw string) (string, int) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var containers []json.Delim
	previous := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				if quote := strings.IndexByte(raw[previous:], '"'); quote >= 0 {
					start := previous + quote
					if reason, offset := deepSeekBufferedSurrogateFailure(raw[start:]); reason != "" {
						return reason, start + offset
					}
					return "EOF while parsing a string", len(raw)
				}
				return "EOF while parsing a value", len(raw)
			}
			if errors.Is(err, io.EOF) && (len(containers) != 0 || previous == 0) {
				kind := "a value"
				trimmed := strings.TrimRight(raw, " \t\r\n")
				if len(containers) != 0 && !strings.HasSuffix(trimmed, ":") && !strings.HasSuffix(trimmed, ",") {
					kind = "a list"
					if containers[len(containers)-1] == '{' {
						kind = "an object"
					}
				}
				return "EOF while parsing " + kind, len(raw)
			}
			return "", 0
		}
		end := int(decoder.InputOffset())
		switch value := token.(type) {
		case json.Number:
			if !jsonutil.CheckSerdeNumberTokens([]byte(value)) {
				return "number out of range", end
			}
		case string:
			if quote := strings.IndexByte(raw[previous:end], '"'); quote >= 0 {
				start := previous + quote
				if reason, offset := deepSeekBufferedSurrogateFailure(raw[start:end]); reason != "" {
					return reason, start + offset
				}
			}
		case json.Delim:
			if value == '[' || value == '{' {
				containers = append(containers, value)
				if len(containers) == 128 {
					return "recursion limit exceeded", end
				}
			} else {
				containers = containers[:len(containers)-1]
			}
		}
		previous = end
	}
}

// Go proved escape syntax through this token or its EOF. Preserve serde's first
// surrogate error and the bytes consumed while expecting a second escape.
func deepSeekBufferedSurrogateFailure(quoted string) (string, int) {
	for index := 1; index+1 < len(quoted); index++ {
		if quoted[index] != '\\' {
			continue
		}
		if quoted[index+1] != 'u' || index+6 > len(quoted) {
			index++
			continue
		}
		unit, _ := strconv.ParseUint(quoted[index+2:index+6], 16, 16)
		switch {
		case unit >= 0xdc00 && unit <= 0xdfff:
			return "lone leading surrogate in hex escape", index + 6
		case unit >= 0xd800 && unit <= 0xdbff:
			next := index + 6
			if next == len(quoted) {
				return "", 0
			}
			if quoted[next] != '\\' {
				return "unexpected end of hex escape", next + 1
			}
			if next+1 == len(quoted) {
				return "", 0
			}
			if quoted[next+1] != 'u' {
				return "unexpected end of hex escape", next + 2
			}
			if next+6 > len(quoted) {
				return "", 0
			}
			nextUnit, _ := strconv.ParseUint(quoted[index+8:index+12], 16, 16)
			if nextUnit < 0xdc00 || nextUnit > 0xdfff {
				return "lone leading surrogate in hex escape", index + 12
			}
			index += 11
		default:
			index += 5
		}
	}
	return "", 0
}
