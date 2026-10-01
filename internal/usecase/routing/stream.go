package routing

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Defer only startup metadata. Output, unknown events, or the byte ceiling commit
// the original stream; a later failure can never trigger another account.
func (router *Router) inspectStream(response *proxymodel.Response, pending *pendingResponse) (responseOutcome, *pendingResponse, error) {
	decoder := sse.NewDecoder(int(router.maxInspect))
	buffer := make([]byte, 4096)
	emptyReads := 0
	for int64(len(pending.prefix)) < router.maxInspect {
		count, readErr := readStreamPrefix(response.Body, buffer, router.maxInspect-int64(len(pending.prefix)))
		pending.prefix = append(pending.prefix, buffer[:count]...)
		if readErr != nil && readErr != io.EOF {
			return responseOutcome{}, pending, readErr
		}
		if outcome, done := startupStreamOutcome(decoder, buffer[:count], response.Header, router.now()); done {
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

func startupStreamOutcome(decoder *sse.Decoder, chunk []byte, headers http.Header, now time.Time) (responseOutcome, bool) {
	for _, data := range decoder.Feed(chunk) {
		outcome, wait := streamOutcome(data, headers, now)
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

func streamOutcome(data []byte, headers http.Header, now time.Time) (responseOutcome, bool) {
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
		if isQuotaResponse(data) {
			return responseOutcome{kind: responseRetry, quarantine: retryAfter(headers, now), failed: true}, false
		}
		return responseOutcome{kind: responsePass, failed: true}, false
	default:
		return responseOutcome{kind: responsePass}, false
	}
}
