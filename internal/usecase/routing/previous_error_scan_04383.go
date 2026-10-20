package routing

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// The tagged 0.438.3 runtime classifies previous-response errors using a
// serde_json::Value followed by an ASCII-only compiled Mojo decision. Code,
// type and param are exact case-sensitive bytes; only message matching and
// the standalone text fallback use ASCII-insensitive comparisons.
const (
	previousErrorNone04383 = iota
	previousErrorNotFound04383
	previousErrorInvalidID04383
	previousErrorToolContext04383
	previousResponseJSONScanLimit04383 = 2048
)

func invalidPreviousResponseID(body []byte) bool {
	value, valid := decodeNativeUsageJSON(body)
	return valid && previousResponseJSONFind04383(value, func(object map[string]any) bool {
		return previousResponseErrorClass04383(object) == previousErrorInvalidID04383
	})
}

func previousResponseNotFound(body []byte) bool {
	// A valid structured previous-response error takes precedence. Rust then
	// checks the complete original text for narrowly defined standalone
	// messages, never partial substring evidence from an invalid JSON body.
	if value, valid := decodeNativeUsageJSON(body); valid && previousResponseMessageInJSON04383(value) {
		return true
	}
	if !utf8.Valid(body) {
		return false
	}
	return previousResponseMessageInText04383(strings.TrimSpace(string(body)))
}

func previousResponseMessageInJSON04383(value any) bool {
	return previousResponseJSONFind04383(value, func(object map[string]any) bool {
		return previousResponseErrorClass04383(object) != previousErrorNone04383
	})
}

func previousResponseJSONFind04383(value any, candidate func(map[string]any) bool) bool {
	// Tagged runtime_json_find evaluates no more than 2048 nodes in a
	// depth-first walk. serde_json's BTreeMap order is lexical for object
	// keys; Go maps must be sorted before pushing children in reverse.
	stack := []any{value}
	for visited := 0; len(stack) > 0 && visited < previousResponseJSONScanLimit04383; visited++ {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		switch typed := current.(type) {
		case map[string]any:
			if candidate(typed) {
				return true
			}
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for index := len(keys) - 1; index >= 0; index-- {
				stack = append(stack, typed[keys[index]])
			}
		case []any:
			for index := len(typed) - 1; index >= 0; index-- {
				stack = append(stack, typed[index])
			}
		}
	}
	return false
}

func previousResponseErrorClass04383(object map[string]any) int {
	code, _ := object["code"].(string)
	if code == "previous_response_not_found" {
		return previousErrorNotFound04383
	}
	typ, _ := object["type"].(string)
	param, _ := object["param"].(string)
	message, present := object["message"].(string)
	if !present {
		message, present = object["detail"].(string)
		if !present {
			message, _ = object["error"].(string)
		}
	}
	if typ == "invalid_request_error" && previousASCIIEqualFold04383(strings.Trim(message, " \t\r\n"), "invalid `previous_response_id`.") {
		return previousErrorInvalidID04383
	}
	if previousResponseToolMessage04383(message) && (typ == "invalid_request_error" || param == "input") {
		return previousErrorToolContext04383
	}
	return previousErrorNone04383
}

func previousResponseMessageInText04383(value string) bool {
	if previousASCIIEqualFold04383(value, "invalid `previous_response_id`.") ||
		previousASCIIPrefix04383(value, "previous_response_not_found") {
		return true
	}
	if previousASCIIPrefix04383(value, "previous response") && previousASCIIContains04383(value, "not found") {
		return true
	}
	return previousResponseToolMessage04383(value) &&
		(previousASCIIPrefix04383(value, "invalid_request_error:") ||
			previousASCIIPrefix04383(value, "no tool call found") ||
			previousASCIIPrefix04383(value, "no function call found"))
}

func previousResponseToolMessage04383(value string) bool {
	return previousASCIIContains04383(value, "no tool call found") ||
		previousASCIIContains04383(value, "no function call found")
}

func previousASCIIContains04383(source, needle string) bool {
	for at := 0; at+len(needle) <= len(source); at++ {
		if previousASCIIEqualFold04383(source[at:at+len(needle)], needle) {
			return true
		}
	}
	return false
}

func previousASCIIPrefix04383(source, prefix string) bool {
	return len(source) >= len(prefix) && previousASCIIEqualFold04383(source[:len(prefix)], prefix)
}

func previousASCIIEqualFold04383(source, literal string) bool {
	if len(source) != len(literal) {
		return false
	}
	for at := 0; at < len(source); at++ {
		value := source[at]
		if value >= 'A' && value <= 'Z' {
			value += 'a' - 'A'
		}
		if value != literal[at] {
			return false
		}
	}
	return true
}
