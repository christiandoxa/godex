package routing

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const (
	nativeUsageScanDepth04383 = 256
	nativeUsageScanNodes04383 = 2048
)

// The Rust response parser requires exactly one complete JSON value.
// Go's Decoder permits concatenated values unless we verify EOF explicitly.
func decodeNativeUsageJSON(payload []byte) (any, bool) {
	if !validNativeJSONUnicode04383(payload) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || !validNativeJSONContainerDepth04383(value) || !validNativeJSONNumberRange04383(value) {
		return nil, false
	}
	return value, true
}

// serde_json::Value rejects non-finite numeric literals anywhere in the
// document, even when Mojo would find usage before encountering them.
// Decoder.UseNumber preserves U64 precision but needs this extra range gate.
func validNativeJSONNumberRange04383(value any) bool {
	switch typed := value.(type) {
	case json.Number:
		literal := string(typed)
		if !strings.ContainsAny(literal, ".eE") && literal != "-0" {
			if literal[0] == '-' {
				if _, err := strconv.ParseInt(literal, 10, 64); err == nil {
					return true
				}
			} else if _, err := strconv.ParseUint(literal, 10, 64); err == nil {
				return true
			}
		}
		// The pinned serde_json default f64 parser does NOT always share
		// Go strconv.ParseFloat's overflow boundary. For example,
		// 1.7976931348623158e308 is finite in Go but rejected by Rust's
		// u64-significand multiplied by an exact literal 10^n constant.
		// Reuse the already verified 0.438.3 serde float projection for
		// entire-document validity, including unrelated JSON members.
		_, valid := quotaSerdeFloat04383(literal)
		return valid
	case map[string]any:
		for _, member := range typed {
			if !validNativeJSONNumberRange04383(member) {
				return false
			}
		}
	case []any:
		for _, item := range typed {
			if !validNativeJSONNumberRange04383(item) {
				return false
			}
		}
	}
	return true
}

// Prodex 0.438.3's compiled response metadata scans usage under arbitrary
// object and array wrappers. Serde JSON serializes object keys in sorted
// order before Mojo scans them; preserve that ordering and the node budget.
func scanNativeUsage(value any) (runtimemodel.TokenUsageCounts, bool) {
	visited := 0
	return scanNativeUsageValue(value, 0, &visited)
}

func scanNativeUsageValue(value any, depth int, visited *int) (runtimemodel.TokenUsageCounts, bool) {
	if depth > nativeUsageScanDepth04383 || *visited >= nativeUsageScanNodes04383 {
		return runtimemodel.TokenUsageCounts{}, false
	}
	*visited++
	switch typed := value.(type) {
	case map[string]any:
		if usage, ok := typed["usage"].(map[string]any); ok {
			if counts, valid := nativeTokenUsageCounts(usage); valid {
				return counts, true
			}
		}
		if counts, valid := nativeTokenUsageCounts(typed); valid {
			return counts, true
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if counts, valid := scanNativeUsageValue(typed[key], depth+1, visited); valid {
				return counts, true
			}
			if *visited >= nativeUsageScanNodes04383 {
				break
			}
		}
	case []any:
		for _, item := range typed {
			if counts, valid := scanNativeUsageValue(item, depth+1, visited); valid {
				return counts, true
			}
			if *visited >= nativeUsageScanNodes04383 {
				break
			}
		}
	}
	return runtimemodel.TokenUsageCounts{}, false
}

// The compiled metadata scanner accepts the full unsigned 64-bit range,
// including JSON -0, but rejects fractional, negative and overflow values.
func unsignedJSONNumber(value any) (uint64, bool) {
	switch current := value.(type) {
	case float64:
		if current >= 0 && current < math.Exp2(64) && math.Trunc(current) == current {
			return uint64(current), true
		}
	case json.Number:
		if current.String() == "-0" {
			return 0, true
		}
		parsed, err := strconv.ParseUint(current.String(), 10, 64)
		if err == nil {
			return parsed, true
		}
	case uint64:
		return current, true
	case int:
		if current >= 0 {
			return uint64(current), true
		}
	}
	return 0, false
}
