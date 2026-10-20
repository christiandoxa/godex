package routing

import (
	"sort"
	"time"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
)

// Prodex's tagged RUNTIME_ERROR_MODE_STREAM Mojo scanner only considers
// literal code, type, reason and status string members. A generic provider's
// error_code aliases, an opaque scalar error or an unrelated note do not
// authorize retrying an OpenAI Responses stream. This walker receives an
// already serde-compatible parsed value, preserving decoded JSON escapes.
const (
	nativeStreamNone04383 = iota
	nativeStreamProfile04383
	nativeStreamOverload04383
	nativeStreamQuota04383
	nativeStreamRate04383
)

func nativeStreamErrorClass04383(value any) (providerentity.ErrorClassification, bool, bool) {
	class, valid := nativeStreamErrorNode04383(value, 0)
	if !valid {
		return providerentity.ErrorClassification{Class: providerentity.ErrorOther}, false, false
	}
	switch class {
	case nativeStreamQuota04383:
		return providerentity.ErrorClassification{Class: providerentity.ErrorQuota, Cooldown: 5 * time.Minute}, false, true
	case nativeStreamRate04383:
		return providerentity.ErrorClassification{Class: providerentity.ErrorRateLimit, Cooldown: time.Minute}, false, true
	case nativeStreamOverload04383:
		return providerentity.ErrorClassification{Class: providerentity.ErrorTransient, Cooldown: 10 * time.Second}, false, true
	case nativeStreamProfile04383:
		return providerentity.ErrorClassification{Class: providerentity.ErrorOther}, true, true
	default:
		return providerentity.ErrorClassification{Class: providerentity.ErrorOther}, false, true
	}
}

func nativeStreamErrorNode04383(value any, depth int) (int, bool) {
	if depth > prodex04383HTTPErrorMaxDepth {
		return nativeStreamNone04383, false
	}
	switch item := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		nested := nativeStreamNone04383
		direct := nativeStreamNone04383
		for _, key := range keys {
			child := item[key]
			if code, ok := child.(string); ok &&
				(key == "code" || key == "type" || key == "status" || key == "reason") {
				// The Mojo direct-record priority is profile, overload,
				// quota and finally rate. All predicates use ASCII bytes.
				if candidate := nativeStreamErrorCode04383(code); candidate != nativeStreamNone04383 &&
					(direct == nativeStreamNone04383 || candidate < direct) {
					direct = candidate
				}
			}
			childClass, valid := nativeStreamErrorNode04383(child, depth+1)
			if !valid {
				return nativeStreamNone04383, false
			}
			if nested == nativeStreamNone04383 && childClass != nativeStreamNone04383 {
				nested = childClass
			}
		}
		if direct != nativeStreamNone04383 {
			return direct, true
		}
		return nested, true
	case []any:
		found := nativeStreamNone04383
		for _, child := range item {
			childClass, valid := nativeStreamErrorNode04383(child, depth+1)
			if !valid {
				return nativeStreamNone04383, false
			}
			if found == nativeStreamNone04383 {
				found = childClass
			}
		}
		return found, true
	default:
		return nativeStreamNone04383, true
	}
}

func nativeStreamErrorCode04383(code string) int {
	switch {
	case previousASCIIEqualFold04383(code, "deactivated_workspace"):
		return nativeStreamProfile04383
	case previousASCIIEqualFold04383(code, "server_is_overloaded"):
		return nativeStreamOverload04383
	case quotaCode(code):
		return nativeStreamQuota04383
	case previousASCIIEqualFold04383(code, "rate_limit_exceeded"),
		previousASCIIEqualFold04383(code, "rate_limit_exceeded_error"),
		previousASCIIEqualFold04383(code, "slow_down"):
		return nativeStreamRate04383
	default:
		return nativeStreamNone04383
	}
}
