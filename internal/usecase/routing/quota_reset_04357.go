package routing

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const (
	prodex04357QuotaQuarantineFallback   = 5 * time.Minute
	prodex04357QuotaKnownResetStaleGrace = 30 * time.Minute
)

func quotaResetAtFromMessage(message []byte, now time.Time) int64 {
	if resetAt, ok := quotaResetAtFromJSON(message); ok {
		return resetAt
	}
	resetAt, _ := quotaResetAtFromText(string(message), now)
	return resetAt
}

func quotaResetAtFromJSON(message []byte) (int64, bool) {
	message = bytes.TrimSpace(message)
	if len(message) == 0 {
		return 0, false
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(message, &root); err != nil || root == nil {
		return 0, false
	}
	for _, key := range []string{"resets_at", "reset_at"} {
		if raw, ok := root[key]; ok {
			if value, valid := quotaResetJSONInt64(raw); valid {
				return value, true
			}
		}
	}
	if raw, ok := root["error"]; ok {
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) == nil && nested != nil {
			for _, key := range []string{"resets_at", "reset_at"} {
				if candidate, found := nested[key]; found {
					if value, valid := quotaResetJSONInt64(candidate); valid {
						return value, true
					}
				}
			}
		}
	}
	rawHeaders, ok := root["headers"]
	if !ok {
		return 0, false
	}
	var headers map[string]json.RawMessage
	if json.Unmarshal(rawHeaders, &headers) != nil || headers == nil {
		return 0, false
	}
	primaryReset, primaryResetOK := quotaResetHeaderInt64(headers, "X-Codex-Primary-Reset-At")
	secondaryReset, secondaryResetOK := quotaResetHeaderInt64(headers, "X-Codex-Secondary-Reset-At")
	primaryUsed, primaryUsedOK := quotaResetHeaderInt64(headers, "X-Codex-Primary-Used-Percent")
	secondaryUsed, secondaryUsedOK := quotaResetHeaderInt64(headers, "X-Codex-Secondary-Used-Percent")

	switch {
	case primaryUsedOK && primaryUsed >= 100:
		if primaryResetOK {
			return primaryReset, true
		}
		return 0, false
	case secondaryUsedOK && secondaryUsed >= 100:
		if secondaryResetOK {
			return secondaryReset, true
		}
		return 0, false
	case primaryResetOK:
		return primaryReset, true
	case secondaryResetOK:
		return secondaryReset, true
	default:
		return 0, false
	}
}

func quotaResetHeaderInt64(headers map[string]json.RawMessage, wanted string) (int64, bool) {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.EqualFold(key, wanted) {
			return quotaResetJSONInt64(headers[key])
		}
	}
	return 0, false
}

func quotaResetJSONInt64(raw json.RawMessage) (int64, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0, false
	}
	if text[0] == '"' {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return 0, false
		}
		text = strings.TrimSpace(value)
	}
	if text == "" {
		return 0, false
	}
	for index, r := range text {
		if index == 0 && (r == '+' || r == '-') {
			continue
		}
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	if text == "+" || text == "-" {
		return 0, false
	}
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil
}

func quotaResetAtFromText(message string, now time.Time) (int64, bool) {
	lower := strings.ToLower(message)
	const marker = "try again at "
	index := strings.Index(lower, marker)
	if index < 0 {
		return 0, false
	}
	candidate := strings.TrimSpace(message[index+len(marker):])
	candidate = strings.TrimSpace(strings.TrimSuffix(candidate, "."))
	parts := strings.Fields(candidate)
	if len(parts) >= 2 {
		if parsed, err := time.ParseInLocation("3:04 PM", strings.Join(parts[:2], " "), now.Location()); err == nil {
			when := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location())
			if !when.After(now) {
				when = when.AddDate(0, 0, 1)
			}
			return when.Unix(), true
		}
	}
	if len(parts) < 5 {
		return 0, false
	}
	day := strings.TrimSuffix(parts[1], ",")
	end := 0
	for end < len(day) && day[end] >= '0' && day[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	parts[1] = day[:end] + ","
	normalized := strings.Join(parts[:5], " ")
	parsed, err := time.ParseInLocation("Jan 2, 2006 3:04 PM", normalized, now.Location())
	if err != nil {
		return 0, false
	}
	return parsed.Unix(), true
}

func (router *Router) quotaQuarantineDuration(accountID string, selection quotamodel.Selection, resetAt int64) time.Duration {
	now := router.now()
	if resetAt > now.Unix() {
		return time.Unix(resetAt, 0).Sub(now)
	}
	if duration, ok := router.knownQuotaResetDuration(accountID, selection, now); ok {
		return duration
	}
	return prodex04357QuotaQuarantineFallback
}

func (router *Router) knownQuotaResetDuration(accountID string, selection quotamodel.Selection, now time.Time) (time.Duration, bool) {
	router.mu.Lock()
	state, ok := router.quotaChecks[quotaCheckKey{accountID: accountID, selection: selection}]
	router.mu.Unlock()
	if !ok || state.checkedAt.After(now) || now.Sub(state.checkedAt) > prodex04357QuotaKnownResetStaleGrace ||
		!state.retryAt.After(now) {
		return 0, false
	}
	return state.retryAt.Sub(now), true
}
