package routing

import (
	"bytes"
	"context"
	"encoding/json"
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
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value map[string]any
	if decoder.Decode(&value) != nil {
		return runtimemodel.TokenUsageCounts{}, false
	}

	eventType, _ := value["type"].(string)
	if eventType != "" &&
		eventType != "response.completed" &&
		eventType != "response.done" &&
		eventType != "response.failed" &&
		eventType != "response.incomplete" {
		return runtimemodel.TokenUsageCounts{}, false
	}

	usage := tokenUsageObject(value)
	if usage == nil {
		return runtimemodel.TokenUsageCounts{}, false
	}
	return nativeTokenUsageCounts(usage)
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

func tokenUsageObject(value map[string]any) map[string]any {
	if usage, ok := value["usage"].(map[string]any); ok {
		if _, valid := nativeTokenUsageCounts(usage); valid {
			return usage
		}
	}
	if _, valid := nativeTokenUsageCounts(value); valid {
		return value
	}
	for _, key := range []string{"response", "data", "result", "payload"} {
		if nested, ok := value[key].(map[string]any); ok {
			if usage := tokenUsageObject(nested); usage != nil {
				return usage
			}
		}
	}
	return nil
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
