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
		remaining := min(int64(len(buffer)), router.maxInspect-int64(len(pending.prefix)))
		count, err := response.Body.Read(buffer[:remaining])
		pending.prefix = append(pending.prefix, buffer[:count]...)
		if err != nil && err != io.EOF {
			return responseOutcome{}, pending, err
		}
		for _, data := range decoder.Feed(buffer[:count]) {
			outcome, wait := streamOutcome(data, response.Header, router.now())
			if !wait {
				return outcome, pending, nil
			}
		}
		if err == io.EOF {
			break
		}
		if count == 0 {
			emptyReads++
			if emptyReads >= 100 {
				return responseOutcome{}, pending, io.ErrNoProgress
			}
		} else {
			emptyReads = 0
		}
	}
	return responseOutcome{kind: responsePass}, pending, nil
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
