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
	var event struct {
		Type  string          `json:"type"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &event) != nil {
		return responseOutcome{kind: responsePass}, false
	}
	event.Type = strings.TrimSpace(event.Type)
	switch event.Type {
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
	if invalidPreviousResponseID(data) {
		return responseOutcome{kind: responsePass, failed: true, invalidPreviousResponseID: true}, false
	}
	classification := providerentity.ClassifyError(http.StatusOK, data)
	switch classification.Class {
	case providerentity.ErrorQuota:
		return responseOutcome{
			kind: responseRetry, quarantine: maxDuration(retryAfter(headers, now), classification.Cooldown),
			failed: true, quota: true, quotaResetAt: quotaResetAtFromMessage(data, now), firstEventRetry: true,
		}, false
	case providerentity.ErrorRateLimit:
		cooldown := maxDuration(retryAfter(headers, now), classification.Cooldown)
		if !externalProviderKind(providerKind) {
			cooldown = rateLimitCooldown(headers, data)
		}
		return responseOutcome{
			kind: responseRetry, quarantine: cooldown,
			failed: true, transient: true, firstEventRetry: true,
		}, false
	case providerentity.ErrorTransient:
		return responseOutcome{
			kind: responseRetry, failed: true, transient: true, healthPenalty: 2, firstEventRetry: true,
		}, false
	}
	if !externalProviderKind(providerKind) && openAIProfileUnavailable(data) {
		return responseOutcome{
			kind: responseRetry, failed: true, profileUnavailable: true, firstEventRetry: true,
		}, false
	}
	if isQuotaResponse(data) {
		return responseOutcome{
			kind: responseRetry, quarantine: retryAfter(headers, now), failed: true, quota: true,
			quotaResetAt: quotaResetAtFromMessage(data, now), firstEventRetry: true,
		}, false
	}
	if previousResponseNotFound(data) {
		return responseOutcome{kind: responsePass, failed: true, previousResponseNotFound: true}, false
	}
	return responseOutcome{kind: responsePass, failed: true}, false
}
