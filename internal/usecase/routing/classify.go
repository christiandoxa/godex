package routing

import (
	"encoding/json"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type responseKind uint8

const (
	responsePass responseKind = iota
	responseRetry
	responseAuthFailure
)

type responseOutcome struct {
	kind       responseKind
	quarantine time.Duration
}

type pendingResponse struct {
	response  *proxymodel.Response
	prefix    []byte
	accountID string
}

func (pending *pendingResponse) close() {
	if pending != nil && pending.response != nil && pending.response.Body != nil {
		pending.response.Body.Close()
	}
}

func (proxy *Router) classify(response *proxymodel.Response) (responseOutcome, *pendingResponse, error) {
	pending := &pendingResponse{response: response}
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return responseOutcome{kind: responseAuthFailure}, pending, nil
	case response.StatusCode == http.StatusTooManyRequests:
		return responseOutcome{kind: responseRetry, quarantine: retryAfter(response.Header, proxy.now())}, pending, nil
	case response.StatusCode == http.StatusInternalServerError ||
		response.StatusCode == http.StatusBadGateway ||
		response.StatusCode == http.StatusServiceUnavailable ||
		response.StatusCode == http.StatusGatewayTimeout:
		return responseOutcome{kind: responseRetry}, pending, nil
	case response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusForbidden:
		prefix, complete, err := inspectResponse(response.Body, proxy.maxInspect)
		pending.prefix = prefix
		if err != nil {
			return responseOutcome{}, pending, err
		}
		if complete && isQuotaResponse(prefix) {
			return responseOutcome{kind: responseRetry, quarantine: 30 * time.Second}, pending, nil
		}
	}
	return responseOutcome{kind: responsePass}, pending, nil
}

func retryAfter(headers http.Header, now time.Time) time.Duration {
	value := strings.TrimSpace(headers.Get("Retry-After"))
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		if seconds > int64((24*time.Hour)/time.Second) {
			return 24 * time.Hour
		}
		return clampDuration(time.Duration(seconds) * time.Second)
	}
	if when, err := http.ParseTime(value); err == nil {
		return clampDuration(when.Sub(now))
	}
	return 30 * time.Second
}

func clampDuration(duration time.Duration) time.Duration {
	if duration < time.Second {
		return time.Second
	}
	if duration > 24*time.Hour {
		return 24 * time.Hour
	}
	return duration
}

func inspectResponse(body io.Reader, limit int64) ([]byte, bool, error) {
	if body == nil {
		return nil, true, nil
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return data, false, err
	}
	return data, int64(len(data)) <= limit, nil
}

func isQuotaResponse(body []byte) bool {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	return quotaValue(value)
}

func quotaValue(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		return quotaMapValue(typed)
	case []any:
		return quotaSliceValue(typed)
	}
	return false
}

func quotaMapValue(values map[string]any) bool {
	for key, child := range values {
		if quotaFieldValue(key, child) || quotaValue(child) {
			return true
		}
	}
	return false
}

func quotaSliceValue(values []any) bool {
	for _, child := range values {
		if quotaValue(child) {
			return true
		}
	}
	return false
}

func quotaFieldValue(key string, value any) bool {
	switch key {
	case "code", "type", "error_code", "status":
		text, ok := value.(string)
		return ok && quotaCode(text)
	default:
		return false
	}
}

func quotaCode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "rate_limit_exceeded", "insufficient_quota", "quota_exceeded", "usage_limit_reached":
		return true
	default:
		return false
	}
}
