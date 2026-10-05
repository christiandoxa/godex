package openai

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type rawAdditionalRateLimit struct {
	LimitID         string          `json:"limit_id"`
	LimitIDCamel    string          `json:"limitId"`
	LimitName       string          `json:"limit_name"`
	LimitNameCamel  string          `json:"limitName"`
	MeteredFeature  string          `json:"metered_feature"`
	FeatureCamel    string          `json:"meteredFeature"`
	NormalModelSlug string          `json:"normal_model_slug"`
	ModelSlugCamel  string          `json:"normalModelSlug"`
	RateLimit       json.RawMessage `json:"rate_limit"`
	RateLimitCamel  json.RawMessage `json:"rateLimit"`
	Allowed         *bool           `json:"allowed"`
	LimitReached    *bool           `json:"limit_reached"`
	LimitReachedAlt *bool           `json:"limitReached"`
}

func decodeAdditionalQuotaLimits(content json.RawMessage, indexed map[string]json.RawMessage) ([]quotamodel.AdditionalRateLimit, error) {
	limits, err := decodeAdditionalRateLimits(content)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(indexed))
	for id := range indexed {
		if !strings.EqualFold(strings.TrimSpace(id), "codex") {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		limit, err := decodeAdditionalRateLimit(indexed[id])
		if err != nil {
			return nil, err
		}
		if limit.LimitID == "" {
			limit.LimitID = id
		}
		if !hasAdditionalRateLimit(limits, limit.LimitID) {
			limits = append(limits, limit)
		}
	}
	return limits, nil
}

func decodeAdditionalRateLimits(content json.RawMessage) ([]quotamodel.AdditionalRateLimit, error) {
	if !rawMessagePresent(content) {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(content, &entries); err != nil {
		return nil, errors.New("decode additional quota rate limits")
	}
	limits := make([]quotamodel.AdditionalRateLimit, 0, len(entries))
	for _, entry := range entries {
		limit, err := decodeAdditionalRateLimit(entry)
		if err != nil {
			return nil, err
		}
		limits = append(limits, limit)
	}
	return limits, nil
}

func decodeAdditionalRateLimit(content json.RawMessage) (quotamodel.AdditionalRateLimit, error) {
	var raw rawAdditionalRateLimit
	if err := json.Unmarshal(content, &raw); err != nil {
		return quotamodel.AdditionalRateLimit{}, errors.New("decode additional quota rate limit")
	}
	pairRaw := firstRaw(raw.RateLimit, raw.RateLimitCamel)
	if len(pairRaw) == 0 {
		pairRaw = content
	}
	pair, err := decodeQuotaPair(pairRaw)
	if err != nil {
		return quotamodel.AdditionalRateLimit{}, err
	}
	var pairFields rawQuotaPair
	if err := json.Unmarshal(pairRaw, &pairFields); err != nil {
		return quotamodel.AdditionalRateLimit{}, errors.New("decode additional quota rate limit")
	}
	return quotamodel.AdditionalRateLimit{
		LimitID:         firstString(raw.LimitID, raw.LimitIDCamel, pairFields.LimitID, pairFields.LimitIDCamel),
		LimitName:       firstString(raw.LimitName, raw.LimitNameCamel, pairFields.LimitName, pairFields.LimitNameCamel),
		MeteredFeature:  firstString(raw.MeteredFeature, raw.FeatureCamel, pairFields.Feature, pairFields.FeatureCamel),
		NormalModelSlug: firstString(raw.NormalModelSlug, raw.ModelSlugCamel, pairFields.ModelSlug, pairFields.ModelSlugCamel),
		Allowed:         firstBool(raw.Allowed, pair.Allowed),
		LimitReached:    firstBool(raw.LimitReached, raw.LimitReachedAlt, pair.LimitReached),
		Primary:         pair.Primary,
		Secondary:       pair.Secondary,
	}, nil
}

func hasAdditionalRateLimit(limits []quotamodel.AdditionalRateLimit, id string) bool {
	if id == "" {
		return false
	}
	for _, limit := range limits {
		if strings.EqualFold(strings.TrimSpace(limit.LimitID), strings.TrimSpace(id)) {
			return true
		}
	}
	return false
}

func firstString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
