package routing

import (
	"context"
	"math"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

// The pinned 0.438.3 runtime_response_raw_usage accepts these exact integer
// fields in strict precedence, including a valid zero and a full uint64.
func TestProdex04383NativeTerminalTokenUsageFields(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		want          runtimemodel.TokenUsageCounts
		valid         bool
	}{
		{"direct-native", `{"type":"response.completed","usage":{"input_tokens":28,"cached_input_tokens":17,"input_tokens_details":{"cached_tokens":99},"output_tokens":7,"reasoning_tokens":5,"output_tokens_details":{"reasoning_tokens":8}}}`, runtimemodel.TokenUsageCounts{InputTokens: 28, CachedInputTokens: 17, OutputTokens: 7, ReasoningTokens: 5}, true},
		{"nested-native-cache", `{"type":"response.completed","response":{"usage":{"input_tokens_details":{"cached_input_tokens":23},"prompt_tokens_details":{"cached_tokens":30}}}}`, runtimemodel.TokenUsageCounts{CachedInputTokens: 23}, true},
		{"prompt-details-cache", `{"type":"response.completed","usage":{"prompt_tokens_details":{"cached_tokens":35}}}`, runtimemodel.TokenUsageCounts{CachedInputTokens: 35}, true},
		{"zero-cache-shadows", `{"type":"response.completed","usage":{"input_tokens_details":{"cached_tokens":0},"prompt_tokens_details":{"cached_tokens":99}}}`, runtimemodel.TokenUsageCounts{}, true},
		{"zero-reasoning-shadows", `{"type":"response.completed","usage":{"reasoning_tokens":0,"output_tokens_details":{"reasoning_tokens":9}}}`, runtimemodel.TokenUsageCounts{}, true},
		{"max-uint64", `{"type":"response.completed","usage":{"cached_input_tokens":18446744073709551615}}`, runtimemodel.TokenUsageCounts{CachedInputTokens: math.MaxUint64}, true},
		{"overflow-falls-through", `{"type":"response.completed","usage":{"cached_input_tokens":18446744073709551616,"prompt_tokens_details":{"cached_tokens":33}}}`, runtimemodel.TokenUsageCounts{CachedInputTokens: 33}, true},
		{"root-native-usage", `{"type":"response.completed","input_tokens":16,"cached_input_tokens":11}`, runtimemodel.TokenUsageCounts{InputTokens: 16, CachedInputTokens: 11}, true},
		{"legacy-cache-only-ignored", `{"type":"response.completed","usage":{"prompt_cache_hit_tokens":99}}`, runtimemodel.TokenUsageCounts{}, false},
		{"empty-usage-rejected", `{"type":"response.completed","usage":{}}`, runtimemodel.TokenUsageCounts{}, false},
		{"nonterminal-still-ignored", `{"type":"response.output_text.delta","usage":{"cached_input_tokens":55}}`, runtimemodel.TokenUsageCounts{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, valid := terminalTokenUsage([]byte(tc.payload))
			if got != tc.want || valid != tc.valid {
				t.Fatalf("counts=%+v valid=%t; want=%+v valid=%t", got, valid, tc.want, tc.valid)
			}
		})
	}
}

// The public runtime marker keeps exact unsigned values after metadata
// extraction; unsupported response-only fields cannot forge an observation.
func TestProdex04383NativeUsageMarkersKeepUnsignedCounts(t *testing.T) {
	recorder := &routingMarkerRecorder{}
	router, err := NewRouter(Config{Activity: recorder, Accounts: func(context.Context) ([]proxymodel.Account, error) {
		return []proxymodel.Account{{ID: "synthetic-main", Enabled: true}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	for _, payload := range []string{
		`{"type":"response.completed","usage":{"prompt_cache_hit_tokens":999}}`,
		`{"type":"response.completed","usage":{"cached_input_tokens":18446744073709551615,"reasoning_tokens":17}}`,
	} {
		if err := router.Observe(t.Context(), "synthetic-main", nil, []byte(payload), false); err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.events) != 1 {
		t.Fatalf("unexpected token usage markers: count=%d", len(recorder.events))
	}
	if fields := recorder.events[0].Fields; fields["cached_input_tokens"] != "18446744073709551615" || fields["reasoning_tokens"] != "17" {
		t.Fatalf("native unsigned counts were rounded or discarded: %+v", fields)
	}
}
