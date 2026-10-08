package routing

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"github.com/christiandoxa/godex/internal/helper/sse"
)

func openAI429Classification(body []byte) providerentity.ErrorClassification {
	rate, quota := openAI429StructuredSignals(body)
	switch {
	case rate:
		return providerentity.ErrorClassification{Class: providerentity.ErrorRateLimit, Cooldown: time.Minute}
	case quota:
		return providerentity.ErrorClassification{Class: providerentity.ErrorQuota, Cooldown: 5 * time.Minute}
	case openAIAuthoritativeQuota429(body):
		return providerentity.ErrorClassification{Class: providerentity.ErrorQuota, Cooldown: 5 * time.Minute}
	case generic429NonRetryable(body):
		return providerentity.ErrorClassification{Class: providerentity.ErrorOther}
	default:
		return providerentity.ErrorClassification{Class: providerentity.ErrorRateLimit, Cooldown: time.Minute}
	}
}

func openAI429HeaderClassification(headers http.Header) (providerentity.ErrorClassification, bool) {
	for _, value := range headers.Values("X-Codex-Rate-Limit-Reached-Type") {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "rate_limit_reached":
			return providerentity.ErrorClassification{Class: providerentity.ErrorRateLimit, Cooldown: time.Minute}, true
		case "workspace_owner_credits_depleted", "workspace_member_credits_depleted",
			"workspace_owner_usage_limit_reached", "workspace_member_usage_limit_reached":
			return providerentity.ErrorClassification{Class: providerentity.ErrorQuota, Cooldown: 5 * time.Minute}, true
		case "":
			continue
		default:
			return providerentity.ErrorClassification{}, false
		}
	}
	return providerentity.ErrorClassification{}, false
}

func openAI429StructuredSignals(body []byte) (rate, quota bool) {
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				name := strings.ToLower(strings.TrimSpace(key))
				if name == "code" || name == "type" || name == "status" || name == "reason" {
					if text, ok := child.(string); ok {
						switch strings.ToLower(strings.TrimSpace(text)) {
						case "rate_limit_exceeded", "rate_limit_exceeded_error", "slow_down":
							rate = true
						case "insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded", "project_spend_limit_exceeded", "quota_exhausted", "quota_exceeded", "resource_exhausted", "usage_limit_reached", "usage_not_included", "workspace_member_credits_depleted":
							quota = true
						}
					}
				}
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	var value any
	if json.Unmarshal(body, &value) == nil {
		visit(value)
		return rate, quota
	}
	decoder := sse.NewDecoder(len(body) + 1)
	events := decoder.Feed(body)
	events = append(events, decoder.Finish()...)
	for _, data := range events {
		value = nil
		if json.Unmarshal(data, &value) == nil {
			visit(value)
		}
	}
	return rate, quota
}

func openAIAuthoritativeQuota429(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, "you've hit your usage limit") ||
		strings.Contains(text, "you have hit your usage limit") ||
		strings.Contains(text, "you hit your usage limit")
}

func generic429NonRetryable(body []byte) bool {
	text := strings.ToLower(string(body))
	for _, marker := range []string{
		"invalid_prompt",
		"bio_policy",
		"cyber_policy",
		"content_policy",
		"invalid_request_error",
		"invalid_request",
		"context_length_exceeded",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func openAIProfileUnavailable(body []byte) bool {
	return strings.Contains(strings.ToLower(string(body)), "deactivated_workspace")
}

func openAIWorkspaceQuotaResponse(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, "you've hit your usage limit") ||
		strings.Contains(text, "you have hit your usage limit") ||
		strings.Contains(text, "the usage limit has been reached") ||
		strings.Contains(text, "usage limit has been reached") ||
		(strings.Contains(text, "usage limit") &&
			(strings.Contains(text, "try again at") ||
				strings.Contains(text, "request to your admin") ||
				strings.Contains(text, "more access now"))) ||
		strings.Contains(text, "workspace_member_credits_depleted") ||
		strings.Contains(text, "workspace is out of credits") ||
		(strings.Contains(text, "out of credits") &&
			strings.Contains(text, "workspace owner") &&
			strings.Contains(text, "refill"))
}
