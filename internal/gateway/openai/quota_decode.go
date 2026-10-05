package openai

import (
	"encoding/json"
	"errors"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type rawQuotaUsage struct {
	PlanType                  string                     `json:"plan_type"`
	PlanTypeCamel             string                     `json:"planType"`
	RateLimit                 json.RawMessage            `json:"rate_limit"`
	RateLimits                json.RawMessage            `json:"rateLimits"`
	RateLimitsSnake           json.RawMessage            `json:"rate_limits"`
	RateLimitsByID            map[string]json.RawMessage `json:"rateLimitsByLimitId"`
	RateLimitsByIDSnake       map[string]json.RawMessage `json:"rate_limits_by_limit_id"`
	AdditionalRateLimits      json.RawMessage            `json:"additional_rate_limits"`
	AdditionalRateLimitsCamel json.RawMessage            `json:"additionalRateLimits"`
	OrdinaryUsageAllowed      *bool                      `json:"ordinaryUsageAllowed"`
	OrdinaryUsageAllowedSnake *bool                      `json:"ordinary_usage_allowed"`
	RateLimitReachedType      json.RawMessage            `json:"rate_limit_reached_type"`
	ResetCredits              *rawResetCredits           `json:"rate_limit_reset_credits"`
	ResetCreditsCamel         *rawResetCredits           `json:"rateLimitResetCredits"`
}

type rawResetCredits struct {
	AvailableCount      *int64 `json:"availableCount"`
	AvailableCountSnake *int64 `json:"available_count"`
}

type rawQuotaPair struct {
	Allowed         *bool           `json:"allowed"`
	LimitReached    *bool           `json:"limit_reached"`
	LimitReachedAlt *bool           `json:"limitReached"`
	LimitID         string          `json:"limit_id"`
	LimitIDCamel    string          `json:"limitId"`
	LimitName       string          `json:"limit_name"`
	LimitNameCamel  string          `json:"limitName"`
	Feature         string          `json:"metered_feature"`
	FeatureCamel    string          `json:"meteredFeature"`
	ModelSlug       string          `json:"normal_model_slug"`
	ModelSlugCamel  string          `json:"normalModelSlug"`
	Primary         json.RawMessage `json:"primary"`
	PrimaryWindow   json.RawMessage `json:"primary_window"`
	PrimaryCamel    json.RawMessage `json:"primaryWindow"`
	Secondary       json.RawMessage `json:"secondary"`
	SecondaryWindow json.RawMessage `json:"secondary_window"`
	SecondaryCamel  json.RawMessage `json:"secondaryWindow"`
}

type rawQuotaWindow struct {
	UsedPercent        *int64 `json:"used_percent"`
	UsedPercentCamel   *int64 `json:"usedPercent"`
	ResetAt            *int64 `json:"reset_at"`
	ResetAtCamel       *int64 `json:"resetAt"`
	ResetsAt           *int64 `json:"resetsAt"`
	WindowSeconds      *int64 `json:"limit_window_seconds"`
	WindowSecondsCamel *int64 `json:"limitWindowSeconds"`
	WindowMinutes      *int64 `json:"windowDurationMins"`
}

func decodeQuotaUsage(body []byte) (quotamodel.Usage, error) {
	var raw rawQuotaUsage
	if err := json.Unmarshal(body, &raw); err != nil {
		return quotamodel.Usage{}, errors.New("decode quota response")
	}
	pairRaw := firstRaw(raw.RateLimit, raw.RateLimits, raw.RateLimitsSnake)
	indexed := firstIndexed(raw.RateLimitsByID, raw.RateLimitsByIDSnake)
	if len(indexed) > 0 {
		if codexPair, ok := indexed["codex"]; ok {
			pairRaw = codexPair
		}
	}
	pair, err := decodeQuotaPair(pairRaw)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	plan := raw.PlanType
	if plan == "" {
		plan = raw.PlanTypeCamel
	}
	usage := quotamodel.Usage{
		PlanType: plan, RateLimitPresent: len(pairRaw) > 0,
		Allowed: pair.Allowed, LimitReached: pair.LimitReached,
		Primary: pair.Primary, Secondary: pair.Secondary,
	}
	additional, err := decodeAdditionalQuotaLimits(
		firstRaw(raw.AdditionalRateLimits, raw.AdditionalRateLimitsCamel), indexed,
	)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	usage.AdditionalRateLimits = additional
	if credits := firstResetCredits(raw.ResetCredits, raw.ResetCreditsCamel); credits != nil {
		if available := firstInt64(credits.AvailableCount, credits.AvailableCountSnake); available != nil {
			usage.ResetCredits = &quotamodel.ResetCredits{AvailableCount: *available}
		}
	}
	if allowed := firstBool(raw.OrdinaryUsageAllowed, raw.OrdinaryUsageAllowedSnake); allowed != nil && !*allowed {
		usage.Allowed = allowed
	}
	if rawMessagePresent(raw.RateLimitReachedType) {
		blocked := true
		usage.LimitReached = &blocked
	}
	return usage, nil
}

func decodeQuotaPair(content json.RawMessage) (quotamodel.Usage, error) {
	if len(content) == 0 || string(content) == "null" {
		return quotamodel.Usage{}, nil
	}
	var raw rawQuotaPair
	if err := json.Unmarshal(content, &raw); err != nil {
		return quotamodel.Usage{}, errors.New("decode quota rate limit")
	}
	primary, err := decodeQuotaWindow(firstRaw(raw.PrimaryWindow, raw.PrimaryCamel, raw.Primary))
	if err != nil {
		return quotamodel.Usage{}, err
	}
	secondary, err := decodeQuotaWindow(firstRaw(raw.SecondaryWindow, raw.SecondaryCamel, raw.Secondary))
	if err != nil {
		return quotamodel.Usage{}, err
	}
	return quotamodel.Usage{
		Allowed: raw.Allowed, LimitReached: firstBool(raw.LimitReached, raw.LimitReachedAlt),
		Primary: primary, Secondary: secondary,
	}, nil
}

func decodeQuotaWindow(content json.RawMessage) (*quotamodel.Window, error) {
	if len(content) == 0 || string(content) == "null" {
		return nil, nil
	}
	var raw rawQuotaWindow
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, errors.New("decode quota window")
	}
	seconds := firstInt64(raw.WindowSeconds, raw.WindowSecondsCamel)
	if seconds == nil && raw.WindowMinutes != nil {
		value := *raw.WindowMinutes * 60
		seconds = &value
	}
	return &quotamodel.Window{
		UsedPercent:        firstInt64(raw.UsedPercent, raw.UsedPercentCamel),
		ResetAt:            firstInt64(raw.ResetAt, raw.ResetAtCamel, raw.ResetsAt),
		LimitWindowSeconds: seconds,
	}, nil
}

func firstResetCredits(values ...*rawResetCredits) *rawResetCredits {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstRaw(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(value) > 0 && string(value) != "null" {
			return value
		}
	}
	return nil
}

func firstIndexed(values ...map[string]json.RawMessage) map[string]json.RawMessage {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstBool(values ...*bool) *bool {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstInt64(values ...*int64) *int64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func rawMessagePresent(value json.RawMessage) bool {
	return len(value) > 0 && string(value) != "null"
}
