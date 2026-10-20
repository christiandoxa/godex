package routing

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"github.com/christiandoxa/godex/internal/helper/sse"
)

func openAI429Classification(body []byte) providerentity.ErrorClassification {
	// The compiled Mojo HTTP error ABI rejects oversized raw bodies, but
	// the tagged 0.438.3 *HTTP candidate loop* still takes the generic
	// retryable-429 fallback for a short invalid-UTF8 error body. The
	// executable oracle proves this is an eligible second-owner attempt,
	// not a literal quota code extracted from the broken JSON.
	if len(body) > prodex04383HTTPErrorMaxBytes {
		return providerentity.ErrorClassification{Class: providerentity.ErrorOther}
	}
	if !utf8.Valid(body) {
		return providerentity.ErrorClassification{Class: providerentity.ErrorRateLimit, Cooldown: time.Minute}
	}
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
	for _, raw := range headers.Values("X-Codex-Rate-Limit-Reached-Type") {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		// The tagged 0.438.3 rate_limit_header_matches kernel folds only
		// ASCII capitals. strings.ToLower would also turn Unicode U+212A
		// (KELVIN SIGN) into 'k', forging a workspace quota code.
		switch {
		case previousASCIIEqualFold04383(value, "rate_limit_reached"):
			return providerentity.ErrorClassification{Class: providerentity.ErrorRateLimit, Cooldown: time.Minute}, true
		case previousASCIIEqualFold04383(value, "workspace_owner_credits_depleted"),
			previousASCIIEqualFold04383(value, "workspace_member_credits_depleted"),
			previousASCIIEqualFold04383(value, "workspace_owner_usage_limit_reached"),
			previousASCIIEqualFold04383(value, "workspace_member_usage_limit_reached"):
			return providerentity.ErrorClassification{Class: providerentity.ErrorQuota, Cooldown: 5 * time.Minute}, true
		default:
			// An unknown first nonblank hint cannot borrow a later value.
			return providerentity.ErrorClassification{}, false
		}
	}
	return providerentity.ErrorClassification{}, false
}

const (
	prodex04383HTTPErrorMaxBytes = 65_536
	prodex04383HTTPErrorMaxDepth = 32
)

func openAI429StructuredSignals(body []byte) (rate, quota bool) {
	// The tagged 0.438.3 HTTP error policy is a bounded *raw JSON* Mojo
	// scanner, not serde_json::Value. Its raw-number grammar accepts 1e309
	// outside a relevant error code, but code names/values must have exact
	// ASCII spellings and the scanner rejects nesting deeper than 32.
	if len(body) > prodex04383HTTPErrorMaxBytes || !utf8.Valid(body) {
		return false, false
	}
	if openAI429ScanRawMojo04383(body, 0, &rate, &quota) {
		return rate, quota
	}
	// A correctly framed SSE 429 body has its own data: JSON event(s).
	// Use the same compiled raw-code policy for each complete event.
	decoder := sse.NewDecoder(len(body) + 1)
	events := decoder.Feed(body)
	events = append(events, decoder.Finish()...)
	for _, data := range events {
		var eventRate, eventQuota bool
		if openAI429ScanRawMojo04383(data, 0, &eventRate, &eventQuota) {
			rate = rate || eventRate
			quota = quota || eventQuota
		}
	}
	return rate, quota
}

func openAI429ScanRawMojo04383(data []byte, depth int, rate, quota *bool) bool {
	if depth > prodex04383HTTPErrorMaxDepth {
		return false
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || !json.Valid(data) {
		return false
	}
	switch data[0] {
	case '{':
		decoder := json.NewDecoder(bytes.NewReader(data))
		if _, err := decoder.Token(); err != nil {
			return false
		}
		for decoder.More() {
			// Decoder.InputOffset includes punctuation skipped before the
			// key. Inspect the ORIGINAL key token to reject escaped or
			// case-altered spellings and preserve duplicate JSON members.
			start := int(decoder.InputOffset())
			key, err := decoder.Token()
			if err != nil {
				return false
			}
			rawKey := bytes.TrimLeft(data[start:int(decoder.InputOffset())], ", \t\r\n")
			var child json.RawMessage
			if err := decoder.Decode(&child); err != nil {
				return false
			}
			if keyName, ok := key.(string); ok &&
				bytes.Equal(rawKey, []byte(`"`+keyName+`"`)) &&
				(keyName == "code" || keyName == "type" || keyName == "status" || keyName == "reason") {
				// Mojo compares original quoted bytes, not Unicode-decoded
				// error tags. \u-encoded values never qualify as codes.
				if len(child) >= 2 && child[0] == '"' && child[len(child)-1] == '"' &&
					!bytes.ContainsRune(child, '\\') {
					code := string(child[1 : len(child)-1])
					switch {
					case previousASCIIEqualFold04383(code, "rate_limit_exceeded"),
						previousASCIIEqualFold04383(code, "rate_limit_exceeded_error"),
						previousASCIIEqualFold04383(code, "slow_down"):
						*rate = true
					case previousASCIIEqualFold04383(code, "insufficient_quota"),
						previousASCIIEqualFold04383(code, "credit_balance_exhausted"),
						previousASCIIEqualFold04383(code, "organization_spend_limit_exceeded"),
						previousASCIIEqualFold04383(code, "project_spend_limit_exceeded"),
						previousASCIIEqualFold04383(code, "quota_exhausted"),
						previousASCIIEqualFold04383(code, "quota_exceeded"),
						previousASCIIEqualFold04383(code, "resource_exhausted"),
						previousASCIIEqualFold04383(code, "usage_limit_reached"),
						previousASCIIEqualFold04383(code, "usage_not_included"),
						previousASCIIEqualFold04383(code, "workspace_member_credits_depleted"):
						*quota = true
					}
				}
			}
			if !openAI429ScanRawMojo04383(child, depth+1, rate, quota) {
				return false
			}
		}
		if _, err := decoder.Token(); err != nil {
			return false
		}
	case '[':
		var items []json.RawMessage
		if json.Unmarshal(data, &items) != nil {
			return false
		}
		for _, item := range items {
			if !openAI429ScanRawMojo04383(item, depth+1, rate, quota) {
				return false
			}
		}
	}
	return true
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

// The tagged Mojo runtime_error_contains folds ASCII uppercase bytes only.
// Unicode case folding would accept U+212A KELVIN SIGN as the ASCII 'k' in
// workspace quota/deactivation markers, changing credential rotation.
func lowerASCIIOnly04383(text string) string {
	out := []byte(text)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + ('a' - 'A')
		}
	}
	return string(out)
}

func openAIProfileUnavailable(body []byte) bool {
	return strings.Contains(lowerASCIIOnly04383(string(body)), "deactivated_workspace")
}

func openAIWorkspaceQuotaResponse(body []byte) bool {
	text := lowerASCIIOnly04383(string(body))
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
