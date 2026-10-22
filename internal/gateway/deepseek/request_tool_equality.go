package deepseek

import "encoding/json"

// Schema copies contain canonical Serde numbers; only floating signed zero compares equally across distinct spellings.
func deepSeekToolValuesEqual(left, right any) bool {
	switch value := left.(type) {
	case map[string]any:
		other, ok := right.(map[string]any)
		if !ok || len(value) != len(other) {
			return false
		}
		for key, item := range value {
			candidate, found := other[key]
			if !found || !deepSeekToolValuesEqual(item, candidate) {
				return false
			}
		}
		return true
	case []any:
		other, ok := right.([]any)
		if !ok || len(value) != len(other) {
			return false
		}
		for index, item := range value {
			if !deepSeekToolValuesEqual(item, other[index]) {
				return false
			}
		}
		return true
	case json.Number:
		return value == right || value == "-0.0" && right == json.Number("0.0") || value == "0.0" && right == json.Number("-0.0")
	default:
		return left == right
	}
}
