package routing

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

// Prodex 0.438.3's runtime_response_raw_usage is a presence-sensitive
// unsigned-u64 parser. Cached tokens use these exact four supported sources
// in precedence order; a valid zero shadows lower-priority positive values.
func TestProdex04383PromptCacheUsageMatchesCompiledUnsignedFields(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      uint64
	}{
		{"direct", `{"usage":{"cached_input_tokens":7}}`, 7},
		{"input-details-cached", `{"usage":{"input_tokens_details":{"cached_tokens":9}}}`, 9},
		{"input-details-native", `{"usage":{"input_tokens_details":{"cached_input_tokens":11}}}`, 11},
		{"prompt-details", `{"usage":{"prompt_tokens_details":{"cached_tokens":13}}}`, 13},
		{"direct-precedence", `{"usage":{"cached_input_tokens":4,"input_tokens_details":{"cached_tokens":8}}}`, 4},
		{"zero-precedence", `{"usage":{"cached_input_tokens":0,"input_tokens_details":{"cached_tokens":8}}}`, 0},
		{"invalid-direct-fallback", `{"usage":{"cached_input_tokens":-2,"input_tokens_details":{"cached_tokens":6}}}`, 6},
		{"nested", `{"response":{"usage":{"cached_input_tokens":15}}}`, 15},
		{"max-u64", `{"usage":{"cached_input_tokens":18446744073709551615}}`, math.MaxUint64},
		{"u64-overflow", `{"usage":{"cached_input_tokens":18446744073709551616}}`, 0},
		{"decimal", `{"usage":{"cached_input_tokens":4.5}}`, 0},
		{"negative-zero", `{"usage":{"cached_input_tokens":-0,"input_tokens_details":{"cached_tokens":8}}}`, 0},
		{"legacy-alias-not-native", `{"usage":{"prompt_cache_hit_tokens":123}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder := json.NewDecoder(strings.NewReader(tc.raw))
			decoder.UseNumber()
			var value any
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			if got := promptCacheCachedInputTokens(value); got != tc.want {
				t.Fatalf("compiled cached token fields: got=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestProdex04383UnsignedJSONNumberRejectsFloatOverflowAndParsesU64(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  uint64
		ok    bool
	}{
		{"max-number", json.Number("18446744073709551615"), math.MaxUint64, true},
		{"overflow-number", json.Number("18446744073709551616"), 0, false},
		{"negative-zero", json.Number("-0"), 0, true},
		{"negative", json.Number("-1"), 0, false},
		{"decimal", json.Number("0.5"), 0, false},
		{"float-2pow64", float64(18446744073709551616.0), 0, false},
		{"float-integral", float64(25), 25, true},
		{"float-fractional", float64(25.5), 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := unsignedJSONNumber(tc.value)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("unsigned count=%d valid=%t, want count=%d valid=%t", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestProdex04383PromptCacheUsageObservationChangesOnlyOnValidLargerHit(t *testing.T) {
	now := time.Unix(1760000000, 0)
	router := &Router{now: func() time.Time { return now }, promptCacheBindings: make(map[string]promptCacheBinding)}
	router.rememberPromptCacheOwner("account-a", "cache-shared", now)
	for _, tc := range []struct {
		name, payload, wantOwner string
	}{
		{"raw-provider-alias-not-accepted", `{"usage":{"prompt_cache_hit_tokens":99}}`, "account-a"},
		{"native-direct-cached", `{"response":{"usage":{"cached_input_tokens":45}}}`, "account-b"},
		{"smaller-hit-preserves-owner", `{"usage":{"input_tokens_details":{"cached_input_tokens":42}}}`, "account-b"},
		{"max-u64-hit-promotes-owner", `{"usage":{"cached_input_tokens":18446744073709551615}}`, "account-c"},
		{"u64-overflow-rejected", `{"usage":{"cached_input_tokens":18446744073709551616}}`, "account-c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := "account-b"
			if strings.Contains(tc.name, "max-u64") || strings.Contains(tc.name, "overflow") {
				account = "account-c"
			}
			router.observePromptCachePayload(account, "cache-shared", []byte(tc.payload))
			if got := router.promptCacheOwner("cache-shared", now); got != tc.wantOwner {
				t.Fatalf("prompt-cache selection after %s = %q, want %q", tc.name, got, tc.wantOwner)
			}
		})
	}
}
