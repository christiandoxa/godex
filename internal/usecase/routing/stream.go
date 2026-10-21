package routing

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Keep Responses startup events replayable until output or a retryable failure.
func (router *Router) inspectStream(response *proxymodel.Response, pending *pendingResponse, providerKind string) (responseOutcome, *pendingResponse, error) {
	return router.inspectStreamWithPolicy(response, pending, providerKind, defaultStreamPrecommitBytes, defaultStreamIdleTimeout)
}

func (router *Router) inspectStreamWithPolicy(
	response *proxymodel.Response,
	pending *pendingResponse,
	providerKind string,
	maxPrecommitBytes int64,
	idleTimeout time.Duration,
) (responseOutcome, *pendingResponse, error) {
	if response.Body == nil {
		return responseOutcome{}, pending, io.ErrUnexpectedEOF
	}
	body := newStreamReadAhead(response.Body, idleTimeout)
	response.Body = body
	if maxPrecommitBytes <= 0 {
		maxPrecommitBytes = defaultStreamPrecommitBytes
	}
	decoder := sse.NewDecoder(int(maxPrecommitBytes))
	buffer := make([]byte, streamReadChunkSize)
	for int64(len(pending.prefix)) < maxPrecommitBytes {
		limit := min(int64(len(buffer)), maxPrecommitBytes-int64(len(pending.prefix)))
		count, readErr := body.Read(buffer[:limit])
		pending.prefix = append(pending.prefix, buffer[:count]...)
		if readErr != nil && readErr != io.EOF {
			return responseOutcome{}, pending, readErr
		}
		if outcome, done := startupStreamOutcome(decoder, buffer[:count], response.Header, router.now(), providerKind); done {
			if outcome.kind == responsePass {
				body.commit()
			}
			return outcome, pending, nil
		}
		if readErr == io.EOF {
			if outcome, done := startupStreamEventsOutcome(decoder.Finish(), response.Header, router.now(), providerKind); done {
				if outcome.kind == responsePass {
					body.commit()
				}
				return outcome, pending, nil
			}
			return responseOutcome{}, pending, io.ErrUnexpectedEOF
		}
	}
	return responseOutcome{}, pending, errStreamPrecommitLimit
}

func startupStreamOutcome(decoder *sse.Decoder, chunk []byte, headers http.Header, now time.Time, providerKind string) (responseOutcome, bool) {
	return startupStreamEventsOutcome(decoder.Feed(chunk), headers, now, providerKind)
}

func startupStreamEventsOutcome(events [][]byte, headers http.Header, now time.Time, providerKind string) (responseOutcome, bool) {
	for _, data := range events {
		outcome, wait := streamOutcome(data, headers, now, providerKind)
		if !wait {
			return outcome, true
		}
	}
	return responseOutcome{}, false
}

func streamOutcome(data []byte, headers http.Header, now time.Time, providerKind string) (responseOutcome, bool) {
	if !utf8.Valid(data) {
		return responseOutcome{}, true
	}
	// Tagged parse_runtime_sse_payload must parse the complete event as
	// serde_json::Value before a structured precommit error can rotate an
	// owner. encoding/json repairs lone surrogates and accepts excessive
	// nesting; neither is legitimate quota/rate-limit evidence.
	if _, valid := decodeNativeUsageJSON(data); !valid {
		return responseOutcome{kind: responsePass}, false
	}
	var event struct {
		Type  string          `json:"type"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &event) != nil {
		return responseOutcome{kind: responsePass}, false
	}
	event.Type = strings.TrimSpace(event.Type)
	if !externalProviderKind(providerKind) {
		// Tagged 0.438.3 runtime_sse_inspection_step checks terminal
		// quota/rate/overload/previous-response flags before precommit_hold.
		// A response.created, in_progress, keepalive or metadata event can
		// therefore expose an authoritative error code without output.
		// Preserve the hold behavior for ordinary metadata without a signal.
		classified, _ := classifyStreamError(data, headers, now, providerKind)
		if classified.kind == responseRetry || classified.invalidPreviousResponseID || classified.previousResponseNotFound {
			return classified, false
		}
	}
	switch event.Type {
	case "keepalive":
		return responseOutcome{}, true
	case "codex.rate_limits",
		"codex.response.metadata",
		"response.metadata",
		"response.created",
		"response.in_progress",
		"response.queued",
		"response.output_item.added",
		"response.content_part.added",
		"response.reasoning_summary_part.added":
		return responseOutcome{}, true
	case "error", "response.failed":
		return classifyStreamError(data, headers, now, providerKind)
	default:
		if len(event.Error) == 0 || bytes.Equal(bytes.TrimSpace(event.Error), []byte("null")) {
			return responseOutcome{kind: responsePass}, false
		}
		return classifyStreamError(data, headers, now, providerKind)
	}
}

func classifyStreamError(data []byte, headers http.Header, now time.Time, providerKind string) (responseOutcome, bool) {
	// The tagged SSE inspection Mojo kernel prioritizes quota/rate/
	// overload failure flags over an *additional* invalid-previous-ID
	// marker carried by the same event. Preserve generic provider ordering
	// on its separate external transport path.
	if externalProviderKind(providerKind) && invalidPreviousResponseID(data) {
		return responseOutcome{kind: responsePass, failed: true, invalidPreviousResponseID: true}, false
	}
	classification := providerentity.ClassifyError(http.StatusOK, data)
	profileUnavailable := false
	if !externalProviderKind(providerKind) {
		parsed, valid := decodeNativeUsageJSON(data)
		if !valid {
			return responseOutcome{kind: responsePass, failed: true}, false
		}
		// OpenAI's 0.438.3 STREAM classifier is not the generic
		// provider error decoder: only canonical code/type/status/reason
		// members in the bounded Mojo tree authorize retry/rotation.
		classification, profileUnavailable, _ = nativeStreamErrorClass04383(parsed)
	}
	switch classification.Class {
	case providerentity.ErrorQuota:
		advice := retryAfter(headers, now)
		if !externalProviderKind(providerKind) {
			advice = nativeRetryAfterHeaders04383(headers)
		}
		return responseOutcome{
			kind: responseRetry, quarantine: maxDuration(advice, classification.Cooldown),
			failed: true, quota: true, quotaResetAt: quotaResetAtFromMessage(data, now), firstEventRetry: true,
		}, false
	case providerentity.ErrorRateLimit:
		cooldown := maxDuration(retryAfter(headers, now), classification.Cooldown)
		if !externalProviderKind(providerKind) {
			cooldown = rateLimitCooldown(headers, data)
		}
		outcome := responseOutcome{
			kind: responseRetry, quarantine: cooldown,
			failed: true, transient: true, firstEventRetry: true,
		}
		if delay, ok := structuredStreamRetryAdvice(data, now); ok {
			outcome.quarantine, outcome.explicitRetryAdvice = delay, true
		}
		return outcome, false
	case providerentity.ErrorTransient:
		outcome := responseOutcome{
			kind: responseRetry, failed: true, transient: true, healthPenalty: 2, firstEventRetry: true,
		}
		if delay, ok := structuredStreamRetryAdvice(data, now); ok {
			outcome.quarantine, outcome.explicitRetryAdvice = delay, true
		}
		return outcome, false
	}
	if profileUnavailable {
		return responseOutcome{
			kind: responseRetry, failed: true, profileUnavailable: true, firstEventRetry: true,
		}, false
	}
	if externalProviderKind(providerKind) && isQuotaResponse(data) {
		return responseOutcome{
			kind: responseRetry, quarantine: retryAfter(headers, now), failed: true, quota: true,
			quotaResetAt: quotaResetAtFromMessage(data, now), firstEventRetry: true,
		}, false
	}
	if invalidPreviousResponseID(data) {
		return responseOutcome{kind: responsePass, failed: true, invalidPreviousResponseID: true}, false
	}
	if previousResponseNotFound(data) {
		return responseOutcome{kind: responsePass, failed: true, previousResponseNotFound: true}, false
	}
	return responseOutcome{kind: responsePass, failed: true}, false
}
