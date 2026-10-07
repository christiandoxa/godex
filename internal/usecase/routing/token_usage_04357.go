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
		eventType != "response.failed" {
		return runtimemodel.TokenUsageCounts{}, false
	}

	usage := tokenUsageObject(value)
	if usage == nil {
		return runtimemodel.TokenUsageCounts{}, false
	}
	cached := tokenUsageDetail(usage, []string{"input_tokens_details", "prompt_tokens_details"}, []string{"cached_tokens"})
	if cached == 0 {
		cached = firstUnsignedUsage(usage, "prompt_cache_hit_tokens")
	}
	return runtimemodel.TokenUsageCounts{
		InputTokens:       firstUnsignedUsage(usage, "input_tokens", "prompt_tokens"),
		CachedInputTokens: cached,
		OutputTokens:      firstUnsignedUsage(usage, "output_tokens", "completion_tokens"),
		ReasoningTokens:   tokenUsageDetail(usage, []string{"output_tokens_details", "completion_tokens_details"}, []string{"reasoning_tokens"}),
	}, true
}

func tokenUsageObject(value map[string]any) map[string]any {
	if usage, ok := value["usage"].(map[string]any); ok {
		return usage
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

func firstUnsignedUsage(object map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		if value, ok := unsignedJSONNumber(object[key]); ok {
			return value
		}
	}
	return 0
}

func tokenUsageDetail(object map[string]any, detailKeys, valueKeys []string) uint64 {
	for _, detailKey := range detailKeys {
		detail, _ := object[detailKey].(map[string]any)
		if detail == nil {
			continue
		}
		if value := firstUnsignedUsage(detail, valueKeys...); value > 0 {
			return value
		}
	}
	return 0
}
