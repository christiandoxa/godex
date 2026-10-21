package routing

import (
	"testing"
	"time"
)

// The 0.438.3 serde_json::Value parser rejects floating-point overflow
// anywhere in a response before Mojo scans usage. Go's Decoder.UseNumber
// accepts that raw notation, so no partial usage/cache evidence may survive.
func TestProdex04383InvalidJSONNumberCannotSupplyUsageEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		valid         bool
	}{
		{"overflow-after-usage", `{"type":"response.completed","usage":{"cached_input_tokens":90},"unrelated":1e309}`, false},
		{"overflow-before-usage", `{"type":"response.completed","unrelated":-1e100000,"usage":{"cached_input_tokens":90}}`, false},
		{"nested-overflow", `{"type":"response.completed","usage":{"cached_input_tokens":90},"data":{"records":[1,2,{"number":1.5e999}]}}`, false},
		{"int-overflow-not-float", `{"type":"response.completed","usage":{"cached_input_tokens":90},"unrelated":18446744073709551616}`, true},
		{"max-supported-exponent", `{"type":"response.completed","usage":{"cached_input_tokens":90},"unrelated":1e308}`, true},
		{"very-small-underflow", `{"type":"response.completed","usage":{"cached_input_tokens":90},"unrelated":1e-99999}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counts, valid := terminalTokenUsage([]byte(tc.payload))
			if valid != tc.valid {
				t.Fatalf("numeric JSON overflow produced usage: %+v valid=%t want=%t", counts, valid, tc.valid)
			}
			if valid && counts.CachedInputTokens != 90 {
				t.Fatalf("valid numeric JSON lost native cached evidence: %+v", counts)
			}
		})
	}
}

func TestProdex04383InvalidJSONNumberCannotChangePromptCacheOwner(t *testing.T) {
	now := time.Unix(1760000000, 0)
	router := &Router{now: func() time.Time { return now }, promptCacheBindings: make(map[string]promptCacheBinding)}
	router.rememberPromptCacheOwner("main", "cache-key", now)
	for _, tc := range []struct {
		name, payload, want string
	}{
		{"overflow", `{"usage":{"cached_input_tokens":91},"ignored":1e309}`, "main"},
		{"negative-overflow", `{"usage":{"cached_input_tokens":92},"ignored":-1e400}`, "main"},
		{"serde-max-float-overflow", `{"usage":{"cached_input_tokens":92},"ignored":1.7976931348623158e308}`, "main"},
		{"serde-max-float-rounddown", `{"usage":{"cached_input_tokens":93},"ignored":1.79769313486231580e308}`, "second"},
		{"finite-large", `{"usage":{"cached_input_tokens":93},"ignored":1e308}`, "second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router.observePromptCachePayload("second", "cache-key", []byte(tc.payload))
			if owner := router.promptCacheOwner("cache-key", now); owner != tc.want {
				t.Fatalf("cached affinity with %s=%q want=%q", tc.name, owner, tc.want)
			}
		})
	}
}
