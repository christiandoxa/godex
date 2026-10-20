package routing

import (
	"context"
	"strconv"
	"strings"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func (router *Router) observeTokenUsage(ctx context.Context, accountID string, payload []byte) {
	if router == nil || router.activity == nil || len(payload) == 0 {
		return
	}
	counts, ok := terminalTokenUsage(payload)
	if !ok {
		return
	}
	fields := map[string]string{
		"input_tokens":        strconv.FormatUint(counts.InputTokens, 10),
		"cached_input_tokens": strconv.FormatUint(counts.CachedInputTokens, 10),
		"output_tokens":       strconv.FormatUint(counts.OutputTokens, 10),
		"reasoning_tokens":    strconv.FormatUint(counts.ReasoningTokens, 10),
	}
	if accountID = strings.TrimSpace(accountID); accountID != "" {
		fields["profile"] = accountID
	}
	router.recordRuntimeMarker(ctx, runtimemodel.Event{
		Kind:      "token_usage",
		AccountID: accountID,
		Fields:    fields,
	})
}

func terminalTokenUsage(payload []byte) (runtimemodel.TokenUsageCounts, bool) {
	value, valid := decodeNativeUsageJSON(payload)
	if !valid {
		return runtimemodel.TokenUsageCounts{}, false
	}
	if object, ok := value.(map[string]any); ok {
		eventType, _ := object["type"].(string)
		if eventType != "" &&
			eventType != "response.completed" &&
			eventType != "response.done" &&
			eventType != "response.failed" &&
			eventType != "response.incomplete" {
			return runtimemodel.TokenUsageCounts{}, false
		}
	}
	return scanNativeUsage(value)
}

// Source-aligned 0.438.3 fields: the first *valid* unsigned value wins,
// including zero. A raw provider cache alias is not native Responses usage.
func nativeTokenUsageCounts(usage map[string]any) (runtimemodel.TokenUsageCounts, bool) {
	input, hasInput := firstUnsignedUsage(usage, "input_tokens", "prompt_tokens")
	cached, hasCached := firstUnsignedUsage(usage, "cached_input_tokens")
	if !hasCached {
		cached, hasCached = tokenUsageDetail(usage, []string{"input_tokens_details"}, "cached_tokens", "cached_input_tokens")
	}
	if !hasCached {
		cached, hasCached = tokenUsageDetail(usage, []string{"prompt_tokens_details"}, "cached_tokens")
	}
	output, hasOutput := firstUnsignedUsage(usage, "output_tokens", "completion_tokens")
	reasoning, hasReasoning := firstUnsignedUsage(usage, "reasoning_tokens")
	if !hasReasoning {
		reasoning, hasReasoning = tokenUsageDetail(usage,
			[]string{"output_tokens_details", "completion_tokens_details"}, "reasoning_tokens")
	}
	return runtimemodel.TokenUsageCounts{
		InputTokens: input, CachedInputTokens: cached, OutputTokens: output, ReasoningTokens: reasoning,
	}, hasInput || hasCached || hasOutput || hasReasoning
}

func firstUnsignedUsage(object map[string]any, keys ...string) (uint64, bool) {
	for _, key := range keys {
		if value, ok := unsignedJSONNumber(object[key]); ok {
			return value, true
		}
	}
	return 0, false
}

func tokenUsageDetail(object map[string]any, detailKeys []string, valueKeys ...string) (uint64, bool) {
	for _, detailKey := range detailKeys {
		if detail, ok := object[detailKey].(map[string]any); ok {
			if value, valid := firstUnsignedUsage(detail, valueKeys...); valid {
				return value, true
			}
		}
	}
	return 0, false
}
