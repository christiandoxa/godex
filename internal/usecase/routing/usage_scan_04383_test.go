package routing

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The official compiled response metadata walks arbitrary object/array
// wrappers, sorted JSON object keys, and bounds that walk to 256 levels and
// 2,048 visited JSON values. Both usage markers and prompt-cache selection
// consume the *same* first discovered usage evidence.
func TestProdex04383NestedUsageScannerMatchesCompiledTraversal(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		want          uint64
		valid         bool
	}{
		{"unknown-object", `{"type":"response.completed","metrics":{"usage":{"cached_input_tokens":71}}}`, 71, true},
		{"array-wrapper", `{"type":"response.completed","events":[{"data":{"usage":{"cached_input_tokens":72}}}]}`, 72, true},
		{"object-key-order", `{"type":"response.completed","zulu":{"usage":{"cached_input_tokens":99}},"alpha":{"usage":{"cached_input_tokens":73}}}`, 73, true},
		{"root-usage-first", `{"type":"response.completed","usage":{"input_tokens":15},"alpha":{"usage":{"cached_input_tokens":74}}}`, 0, true},
		{"direct-root-before-child", `{"type":"response.completed","cached_input_tokens":75,"alpha":{"usage":{"cached_input_tokens":99}}}`, 75, true},
		{"empty-usage-then-child", `{"type":"response.completed","usage":{},"alpha":{"usage":{"cached_input_tokens":76}}}`, 76, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counts, valid := terminalTokenUsage([]byte(tc.payload))
			if valid != tc.valid || counts.CachedInputTokens != tc.want {
				t.Fatalf("usage scan cached=%d valid=%t, want=%d valid=%t", counts.CachedInputTokens, valid, tc.want, tc.valid)
			}
			decoder := json.NewDecoder(strings.NewReader(tc.payload))
			decoder.UseNumber()
			var value any
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			if got := promptCacheCachedInputTokens(value); got != tc.want {
				t.Fatalf("prompt-cache nested scan=%d, want=%d", got, tc.want)
			}
		})
	}
}

func TestProdex04383UsageScanBoundsDepthAndVisitedValues(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		want    uint64
		valid   bool
	}{
		{"depth-125-serde-valid", usageDepthFixture04383(125), 81, true},
		{"depth-126-serde-overflow", usageDepthFixture04383(126), 0, false},
		{"depth-255-mojo-alone-would-accept", usageDepthFixture04383(255), 0, false},
		{"depth-257", usageDepthFixture04383(257), 0, false},
		{"2048-scan-budget", usageBudgetFixture04383(2046), 0, false},
		{"within-scan-budget", usageBudgetFixture04383(4), 82, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counts, valid := terminalTokenUsage([]byte(tc.payload))
			if counts.CachedInputTokens != tc.want || valid != tc.valid {
				t.Fatalf("bounded scanner cached=%d valid=%t want=%d valid=%t", counts.CachedInputTokens, valid, tc.want, tc.valid)
			}
		})
	}
}

func usageDepthFixture04383(depth int) string {
	return `{"type":"response.completed","a":` + strings.Repeat(`{"a":`, depth-1) +
		`{"usage":{"cached_input_tokens":81}}` + strings.Repeat(`}`, depth-1) + `}`
}

func usageBudgetFixture04383(count int) string {
	items := make([]string, count)
	for i := range items {
		items[i] = strconv.Itoa(i)
	}
	return `{"type":"response.completed","a":[` + strings.Join(items, ",") + `],"z":{"usage":{"cached_input_tokens":82}}}`
}

func TestProdex04383UsageScanRejectsTrailingJSONAndReadsRootArrays(t *testing.T) {
	base := `{"type":"response.completed","usage":{"cached_input_tokens":88}}`
	for _, tc := range []struct {
		name, payload string
		want          uint64
		valid         bool
	}{
		{"valid-whitespace", base + " \r\n\t", 88, true},
		{"trailing-garbage", base + `not-json`, 0, false},
		{"second-json-object", base + `{"usage":{"cached_input_tokens":99}}`, 0, false},
		{"second-json-array", base + `[]`, 0, false},
		{"root-array", `[{"response":{"usage":{"cached_input_tokens":89}}}]`, 89, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counts, valid := terminalTokenUsage([]byte(tc.payload))
			if valid != tc.valid || counts.CachedInputTokens != tc.want {
				t.Fatalf("usage payload cached=%d valid=%t want=%d valid=%t", counts.CachedInputTokens, valid, tc.want, tc.valid)
			}
		})
	}
}

func TestProdex04383InvalidTrailingJSONCannotChangePromptCacheOwner(t *testing.T) {
	now := time.Unix(1760000000, 0)
	router := &Router{now: func() time.Time { return now }, promptCacheBindings: make(map[string]promptCacheBinding)}
	router.rememberPromptCacheOwner("main", "cache-key", now)
	router.observePromptCachePayload("second", "cache-key", []byte(`{"usage":{"cached_input_tokens":90}} {"other":true}`))
	if owner := router.promptCacheOwner("cache-key", now); owner != "main" {
		t.Fatalf("invalid JSON changed prompt-cache owner to %q", owner)
	}
	router.observePromptCachePayload("second", "cache-key", []byte(`{"usage":{"cached_input_tokens":90}}`))
	if owner := router.promptCacheOwner("cache-key", now); owner != "second" {
		t.Fatalf("valid JSON did not change owner: %q", owner)
	}
}
