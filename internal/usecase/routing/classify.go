package routing

import (
	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
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
		response.StatusCode == http.StatusGatewayTimeout:
		return responseOutcome{kind: responseRetry, transient: true, healthPenalty: 2}, pending, nil
	case response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusForbidden:
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
		if complete && isQuotaResponse(prefix) {
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
