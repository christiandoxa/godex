package routing

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Defer only startup metadata. Output, unknown events, or the byte ceiling commit
// the original stream; a later failure can never trigger another account.
func (router *Router) inspectStream(response *proxymodel.Response, pending *pendingResponse, providerKind string) (responseOutcome, *pendingResponse, error) {
	decoder := sse.NewDecoder(int(router.maxInspect))
	buffer := make([]byte, 4096)
	emptyReads := 0
	for int64(len(pending.prefix)) < router.maxInspect {
		count, readErr := readStreamPrefix(response.Body, buffer, router.maxInspect-int64(len(pending.prefix)))
		pending.prefix = append(pending.prefix, buffer[:count]...)
		if readErr != nil && readErr != io.EOF {
			return responseOutcome{}, pending, readErr
		}
		if outcome, done := startupStreamOutcome(decoder, buffer[:count], response.Header, router.now(), providerKind); done {
			return outcome, pending, nil
		}
		if readErr == io.EOF {
			break
		}
		if err := trackEmptyRead(count, &emptyReads); err != nil {
			return responseOutcome{}, pending, err
		}
	}
	return responseOutcome{kind: responsePass}, pending, nil
}

func readStreamPrefix(body io.Reader, buffer []byte, remaining int64) (int, error) {
	limit := min(int64(len(buffer)), remaining)
	return body.Read(buffer[:limit])
}

func startupStreamOutcome(decoder *sse.Decoder, chunk []byte, headers http.Header, now time.Time, providerKind string) (responseOutcome, bool) {
	for _, data := range decoder.Feed(chunk) {
		outcome, wait := streamOutcome(data, headers, now, providerKind)
		if !wait {
			return outcome, true
		}
	}
	return responseOutcome{}, false
}

func trackEmptyRead(count int, emptyReads *int) error {
	if count > 0 {
		*emptyReads = 0
		return nil
	}
	*emptyReads++
	if *emptyReads >= 100 {
		return io.ErrNoProgress
	}
	return nil
}

func streamOutcome(data []byte, headers http.Header, now time.Time, providerKind string) (responseOutcome, bool) {
	var event struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &event) != nil {
		return responseOutcome{kind: responsePass}, false
	}
	switch event.Type {
	case "response.created", "response.in_progress":
		return responseOutcome{}, true
	case "error", "response.failed":
		if invalidPreviousResponseID(data) {
			return responseOutcome{kind: responsePass, failed: true, invalidPreviousResponseID: true}, false
		}
		if previousResponseNotFound(data) {
			return responseOutcome{kind: responsePass, failed: true, previousResponseNotFound: true}, false
		}
		classification := providerentity.ClassifyError(http.StatusOK, data)
		switch classification.Class {
		case providerentity.ErrorQuota:
			return responseOutcome{
				kind: responseRetry, quarantine: maxDuration(retryAfter(headers, now), classification.Cooldown),
				failed: true, quota: true, firstEventRetry: true,
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
		if isQuotaResponse(data) {
			return responseOutcome{kind: responseRetry, quarantine: retryAfter(headers, now), failed: true, quota: true, firstEventRetry: true}, false
		}
		return responseOutcome{kind: responsePass, failed: true}, false
	default:
		return responseOutcome{kind: responsePass}, false
	}
}
