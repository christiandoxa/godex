package routing

import (
	"encoding/json"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"github.com/christiandoxa/godex/internal/helper/sse"
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
	kind                      responseKind
	quarantine                time.Duration
	failed                    bool
	quota                     bool
	profileUnavailable        bool
	transient                 bool
	transport                 bool
	healthPenalty             uint8
	firstEventRetry           bool
	invalidPreviousResponseID bool
	previousResponseNotFound  bool
}

type pendingResponse struct {
	response                 *proxymodel.Response
	prefix                   []byte
	accountID                string
	firstEventRetry          bool
	authFailure              bool
	quota                    bool
	profileUnavailable       bool
	transient                bool
	previousResponseNotFound bool
}

func (pending *pendingResponse) close() {
	if pending != nil && pending.response != nil && pending.response.Body != nil {
		pending.response.Body.Close()
	}
}

func (proxy *Router) classify(response *proxymodel.Response, providerKind string) (responseOutcome, *pendingResponse, error) {
	pending := &pendingResponse{response: response}
	if outcome, specialPending, err, handled := proxy.classifySpecialResponse(response, pending, providerKind); handled {
		return outcome, specialPending, err
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return responseOutcome{kind: responseAuthFailure}, pending, nil
	case response.StatusCode == http.StatusTooManyRequests:
		prefix, complete, err := inspectResponse(response.Body, proxy.maxInspect)
		pending.prefix = prefix
		if err != nil {
			return responseOutcome{}, pending, err
		}
		classificationBody := prefix
		if !complete {
			classificationBody = nil
		}
		classification := providerentity.ClassifyError(response.StatusCode, classificationBody)
		if !externalProviderKind(providerKind) {
			classification = openAI429Classification(classificationBody)
		}
		if classification.Class != providerentity.ErrorQuota &&
			classification.Class != providerentity.ErrorRateLimit &&
			classification.Class != providerentity.ErrorTransient {
			return responseOutcome{kind: responsePass}, pending, nil
		}
		cooldown := maxDuration(retryAfter(response.Header, proxy.now()), classification.Cooldown)
		if classification.Class == providerentity.ErrorRateLimit {
			cooldown = rateLimitCooldown(response.Header, classificationBody)
		}
		return responseOutcome{
			kind: responseRetry, quarantine: cooldown,
			quota:         classification.Class == providerentity.ErrorQuota,
			transient:     classification.Class == providerentity.ErrorRateLimit || classification.Class == providerentity.ErrorTransient,
			healthPenalty: transientHealthPenalty(classification.Class),
		}, pending, nil
	case response.StatusCode == http.StatusInternalServerError ||
		response.StatusCode == http.StatusBadGateway ||
		response.StatusCode == http.StatusServiceUnavailable ||
		response.StatusCode == http.StatusGatewayTimeout ||
		response.StatusCode == 529:
		return responseOutcome{kind: responseRetry, transient: true, healthPenalty: 2}, pending, nil
	case response.StatusCode == http.StatusBadRequest ||
		response.StatusCode == http.StatusPaymentRequired ||
		response.StatusCode == http.StatusForbidden:
		prefix, complete, err := inspectResponse(response.Body, proxy.maxInspect)
		pending.prefix = prefix
		if err != nil {
			return responseOutcome{}, pending, err
		}
		if response.StatusCode == http.StatusBadRequest && complete && invalidPreviousResponseID(prefix) {
			return responseOutcome{kind: responsePass, failed: true, invalidPreviousResponseID: true}, pending, nil
		}
		if response.StatusCode == http.StatusBadRequest && complete && previousResponseNotFound(prefix) {
			return responseOutcome{kind: responsePass, failed: true, previousResponseNotFound: true}, pending, nil
		}
		if !externalProviderKind(providerKind) && complete &&
			(response.StatusCode == http.StatusPaymentRequired || response.StatusCode == http.StatusForbidden) &&
			openAIProfileUnavailable(prefix) {
			return responseOutcome{kind: responseRetry, profileUnavailable: true}, pending, nil
		}
		if complete && (isQuotaResponse(prefix) ||
			(!externalProviderKind(providerKind) &&
				(response.StatusCode == http.StatusPaymentRequired || response.StatusCode == http.StatusForbidden) &&
				openAIWorkspaceQuotaResponse(prefix))) {
			return responseOutcome{kind: responseRetry, quarantine: 30 * time.Second, quota: true}, pending, nil
		}
	}
	return responseOutcome{kind: responsePass}, pending, nil
}

func (proxy *Router) classifySpecialResponse(
	response *proxymodel.Response,
	pending *pendingResponse,
	providerKind string,
) (responseOutcome, *pendingResponse, error, bool) {
	switch {
	case response.PrecommitFailure != nil:
		outcome, pending, err := proxy.classifyPrecommitFailure(response, pending)
		return outcome, pending, err, true
	case response.FirstEventCommitted:
		return responseOutcome{}, pending, nil, true
	case response.StatusCode == http.StatusOK &&
		strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") &&
		response.Header.Get("Content-Encoding") == "":
		outcome, pending, err := proxy.inspectStream(response, pending, providerKind)
		if err != nil {
			pending.transient = true
		}
		return outcome, pending, err, true
	case externalProviderKind(providerKind) && response.StatusCode >= http.StatusBadRequest:
		outcome, pending, err := proxy.classifyExternalProvider(response, pending)
		return outcome, pending, err, true
	default:
		return responseOutcome{}, pending, nil, false
	}
}

func (proxy *Router) classifyPrecommitFailure(response *proxymodel.Response, pending *pendingResponse) (responseOutcome, *pendingResponse, error) {
	failure := response.PrecommitFailure
	if strings.EqualFold(strings.TrimSpace(failure.Code), "previous_response_not_found") {
		return responseOutcome{kind: responsePass, failed: true, previousResponseNotFound: true}, pending, nil
	}
	transport := failure.Transport
	if strings.EqualFold(strings.TrimSpace(failure.Code), "deactivated_workspace") {
		if response.FirstEventRetryUsed || response.FirstEventCommitted {
			return responseOutcome{kind: responsePass, failed: true}, pending, nil
		}
		return responseOutcome{
			kind: responseRetry, failed: true, profileUnavailable: true, firstEventRetry: true,
		}, pending, nil
	}
	classification := providerentity.ClassifyProviderCode(failure.Code)
	if transport {
		classification = providerentity.ClassifyError(http.StatusBadGateway, nil)
	}
	if response.FirstEventRetryUsed || response.FirstEventCommitted || !providerentity.RetryableAcrossCredentials(classification.Class) {
		return responseOutcome{kind: responsePass, failed: true, transport: transport}, pending, nil
	}
	if classification.Class == providerentity.ErrorAuth {
		return responseOutcome{kind: responseAuthFailure, failed: true, transport: transport, firstEventRetry: true}, pending, nil
	}
	return responseOutcome{
		kind: responseRetry, quarantine: classification.Cooldown, failed: true,
		quota:           classification.Class == providerentity.ErrorQuota,
		transient:       classification.Class == providerentity.ErrorRateLimit || classification.Class == providerentity.ErrorTransient,
		transport:       transport,
		healthPenalty:   precommitHealthPenalty(classification.Class, transport, failure.Code),
		firstEventRetry: true,
	}, pending, nil
}

func transientHealthPenalty(class providerentity.ErrorClass) uint8 {
	if class == providerentity.ErrorTransient {
		return 2
	}
	return 0
}

func precommitHealthPenalty(class providerentity.ErrorClass, transport bool, transportCode string) uint8 {
	if transport {
		return transportHealthPenalty(transportCode)
	}
	return transientHealthPenalty(class)
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

func maxDuration(left, right time.Duration) time.Duration {
	if right > left {
		return right
	}
	return left
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

func (proxy *Router) classifyExternalProvider(
	response *proxymodel.Response,
	pending *pendingResponse,
) (responseOutcome, *pendingResponse, error) {
	prefix, complete, err := inspectResponse(response.Body, proxy.maxInspect)
	pending.prefix = prefix
	if err != nil {
		return responseOutcome{}, pending, err
	}
	classificationBody := prefix
	if !complete {
		classificationBody = nil
	}
	classification := providerentity.ClassifyError(response.StatusCode, classificationBody)
	switch classification.Class {
	case providerentity.ErrorAuth:
		return responseOutcome{kind: responseAuthFailure}, pending, nil
	case providerentity.ErrorQuota:
		return responseOutcome{kind: responseRetry, quarantine: classification.Cooldown, quota: true}, pending, nil
	case providerentity.ErrorRateLimit:
		return responseOutcome{
			kind: responseRetry, quarantine: maxDuration(retryAfter(response.Header, proxy.now()), classification.Cooldown), transient: true,
		}, pending, nil
	case providerentity.ErrorTransient:
		return responseOutcome{kind: responseRetry, quarantine: classification.Cooldown, transient: true, healthPenalty: 2}, pending, nil
	default:
		return responseOutcome{kind: responsePass}, pending, nil
	}
}

func externalProviderKind(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return kind != "" && kind != "openai"
}

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
		strings.Contains(text, "you hit your usage limit") ||
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
