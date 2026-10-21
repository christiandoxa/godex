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
	// Tagged quota_reset_at_from_json_message first parses the COMPLETE
	// document as serde_json::Value. RawMessage would otherwise accept
	// unrelated invalid Unicode, overflowing numbers or excess depth and
	// turn a bogus reset timestamp into a long-lived profile quarantine.
	if _, valid := decodeNativeUsageJSON(message); !valid {
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
	// The wanted header names are fixed ASCII constants; normalize only
	// this comparator literal, never Unicode-fold the untrusted key.
	wantedLower := strings.ToLower(wanted)
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		// The compiled Mojo quota_json_key_casefold_equal folds ASCII
		// capitals only. Go EqualFold would turn U+017F LONG S into 's'
		// and accept a forged reset/used-percent header name.
		if previousASCIIEqualFold04383(key, wantedLower) {
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
		// Tagged quota_reset_at_from_json_message canonicalizes through
		// serde_json before the Mojo numeric string walker. Tabs/newlines
		// remain escaped inside canonical strings, and Unicode spaces are
		// never ASCII padding. Only a literal ASCII SPACE is valid here.
		text = strings.Trim(value, " ")
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
	const marker = "try again at "
	// Tagged 0.438.3 finds this phrase by ASCII-casefolding the original
	// UTF-8 bytes and returns an offset INTO THOSE ORIGINAL BYTES. Go's
	// Unicode-aware strings.ToLower maps U+0130 to ASCII 'i' and changes
	// byte lengths: it both accepts forged markers and truncates legitimate
	// clock text when non-ASCII characters precede the marker.
	index := quotaASCIICasefoldIndex04383(message, marker)
	if index < 0 {
		return 0, false
	}
	candidate := strings.TrimSpace(message[index+len(marker):])
	candidate = strings.TrimRight(candidate, ".")
	if len(candidate) > 8192 {
		return 0, false
	}
	parts := strings.FieldsFunc(candidate, quotaRetryASCIIWhitespace)
	if len(parts) >= 2 {
		if hour, minute, ok := quotaChronoClock04383(parts[0], parts[1]); ok {
			when, exists := quotaChronoLocalDateTime04383(now.Year(), now.Month(), now.Day(), hour, minute, now.Location())
			if !exists {
				return 0, false
			}
			if !when.After(now) {
				// Source chrono advances the NAIVE calendar day and resolves
				// it again. Go AddDate on a DST-normalized instant can retain
				// the wrong civil hour or accept a nonexistent next-day time.
				civil := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
				when, exists = quotaChronoLocalDateTime04383(civil.Year(), civil.Month(), civil.Day(), hour, minute, now.Location())
				if !exists {
					return 0, false
				}
			}
			return when.Unix(), true
		}
	}
	if len(parts) < 5 {
		return 0, false
	}
	// The pinned Mojo candidate parser normalizes ordinal suffixes, then
	// chrono 0.4.45 accepts month/day/year and an ASCII-insensitive 12h
	// clock. Go time.ParseInLocation("... 3:04 PM") rejects lower/mixed
	// case AM/PM and one-digit minutes, yet accepts hour 0, which chrono
	// rejects. Preserve the strict chrono field ranges before constructing
	// a local date instead of attempting Go's permissive 12h parser.
	day := strings.TrimSuffix(parts[1], ",")
	end := 0
	for end < len(day) && day[end] >= '0' && day[end] <= '9' {
		end++
	}
	if end == 0 || end > 2 {
		return 0, false
	}
	year, validYear := quotaChronoYear04383(parts[2])
	if !validYear {
		return 0, false
	}
	d, _ := strconv.Atoi(day[:end])
	month, err := time.Parse("Jan", parts[0])
	if err != nil {
		return 0, false
	}
	hour, minute, ok := quotaChronoClock04383(parts[3], parts[4])
	if !ok {
		return 0, false
	}
	parsed, valid := quotaChronoLocalDateTime04383(year, month.Month(), d, hour, minute, now.Location())
	if !valid {
		return 0, false
	}
	return parsed.Unix(), true
}

// Tagged chrono::Local.from_local_datetime(...).single().or_else(earliest)
// rejects civil times lost at a spring DST jump. On an ambiguous fall-back
// wall time its pinned LocalResult::earliest selects the standard-time
// offset first (confirmed against chrono 0.4.45 for New York, Berlin and
// Sydney), even when Go time.Date selects the other DST occurrence.
// Enumerate the zone's nearby valid offsets and accept only actual instants
// that reproduce the requested civil fields. Never normalize a DST gap.
func quotaChronoLocalDateTime04383(year int, month time.Month, day, hour, minute int, location *time.Location) (time.Time, bool) {
	wallUTC := time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
	if wallUTC.Year() != year || wallUTC.Month() != month || wallUTC.Day() != day {
		return time.Time{}, false
	}
	wallSeconds := wallUTC.Unix()
	seenOffsets := make(map[int]struct{}, 4)
	var chosen time.Time
	chosenOffset := 0
	found := false
	for hours := -36; hours <= 36; hours += 6 {
		_, offset := time.Unix(wallSeconds+int64(hours)*3600, 0).In(location).Zone()
		if _, seen := seenOffsets[offset]; seen {
			continue
		}
		seenOffsets[offset] = struct{}{}
		candidate := time.Unix(wallSeconds-int64(offset), 0).In(location)
		if candidate.Year() != year || candidate.Month() != month || candidate.Day() != day ||
			candidate.Hour() != hour || candidate.Minute() != minute {
			continue
		}
		if !found || offset < chosenOffset {
			chosen, chosenOffset, found = candidate, offset, true
		}
	}
	return chosen, found
}

// chrono 0.4.45's "%I:%M %p" expects 1..12 (never zero) hours,
// 1- or 2-digit minutes, and ASCII-case-insensitive AM/PM. The tagged
// Mojo planner has already collapsed its first two whitespace tokens.
func quotaChronoClock04383(clock, period string) (int, int, bool) {
	colon := strings.IndexByte(clock, ':')
	if colon < 1 || colon > 2 || len(clock)-colon-1 < 1 || len(clock)-colon-1 > 2 ||
		!quotaASCIIDigits04383(clock[:colon]) || !quotaASCIIDigits04383(clock[colon+1:]) {
		return 0, 0, false
	}
	hour, _ := strconv.Atoi(clock[:colon])
	minute, _ := strconv.Atoi(clock[colon+1:])
	if hour < 1 || hour > 12 || minute > 59 {
		return 0, 0, false
	}
	isAM := previousASCIIEqualFold04383(period, "am")
	isPM := previousASCIIEqualFold04383(period, "pm")
	if !isAM && !isPM {
		return 0, 0, false
	}
	if hour == 12 {
		hour = 0
	}
	if isPM {
		hour += 12
	}
	return hour, minute, true
}

// chrono 0.4.45 parses unsigned `%Y` as 1–4 digits. An explicit +/-
// sign permits an extended proleptic year, bounded by chrono's supported
// NaiveDate range (-262143 through +262142). This must not be silently
// coerced through Go's stricter `time.Parse(..., "2006")` grammar.
func quotaChronoYear04383(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	digits := value
	signed := value[0] == '+' || value[0] == '-'
	if signed {
		digits = value[1:]
	} else if len(digits) > 4 {
		return 0, false
	}
	if !quotaASCIIDigits04383(digits) {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < -262143 || parsed > 262142 {
		return 0, false
	}
	return int(parsed), true
}

func quotaASCIIDigits04383(value string) bool {
	if value == "" {
		return false
	}
	for at := 0; at < len(value); at++ {
		if value[at] < '0' || value[at] > '9' {
			return false
		}
	}
	return true
}

func quotaASCIICasefoldIndex04383(source, literal string) int {
	for at := 0; at+len(literal) <= len(source); at++ {
		if previousASCIIEqualFold04383(source[at:at+len(literal)], literal) {
			return at
		}
	}
	return -1
}

func quotaRetryASCIIWhitespace(value rune) bool {
	switch value {
	case '\t', '\n', '\r', ' ':
		return true
	default:
		return false
	}
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
