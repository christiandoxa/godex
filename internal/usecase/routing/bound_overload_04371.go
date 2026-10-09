package routing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const (
	boundOverloadMaxRetries     = 5
	boundOverloadPlanningWindow = time.Minute
	boundOverloadBaseDelay      = 250 * time.Millisecond
)

// Matches the tagged Prodex 0.437.1 Mojo bounded same-owner retry planner.
// The retry clock is measured from the first precommit attempt and never
// shortened to force upstream Retry-After into a remaining deadline.
func boundOverloadRetryDelay(
	hardAffinity, hasPreviousResponse, committed bool,
	retries int, elapsed, advice time.Duration, advicePresent bool,
	requestID uint64,
) (time.Duration, bool) {
	if !hardAffinity || hasPreviousResponse || committed || retries < 0 ||
		retries >= boundOverloadMaxRetries || elapsed >= boundOverloadPlanningWindow {
		return 0, false
	}
	if elapsed > 0 && elapsed%time.Millisecond != 0 {
		// Rust converts elapsed nanoseconds to milliseconds with ceil.
		elapsed = (elapsed/time.Millisecond + 1) * time.Millisecond
	}
	delay := boundOverloadBaseDelay*time.Duration(1<<uint(retries)) +
		time.Duration(requestID%251)*time.Millisecond
	if advicePresent {
		if advice < 0 {
			advice = 0
		}
		if advice >= boundOverloadPlanningWindow {
			// No 60-second planning window can admit this advice. Check
			// before millisecond ceiling to avoid Duration overflow.
			return 0, false
		}
		// The exact tagged policy rounds supplied durations up to a whole
		// millisecond before comparing to exponential backoff.
		if remainder := advice % time.Millisecond; remainder != 0 {
			advice += time.Millisecond - remainder
		}
		if advice > delay {
			delay = advice
		}
	}
	if delay >= boundOverloadPlanningWindow-elapsed {
		return 0, false
	}
	return delay, true
}

func boundOverloadAdvice(response *proxymodel.Response, pending *pendingResponse, now time.Time) (time.Duration, bool) {
	if response == nil {
		return 0, false
	}
	if response.Header != nil {
		value := response.Header.Get("Retry-After")
		if value != "" {
			if advice, ok := parseStreamRetryHeader(value, now); ok {
				return advice, true
			}
		}
	}
	if pending == nil || len(pending.prefix) == 0 {
		return 0, false
	}
	decoder := sse.NewDecoder(len(pending.prefix))
	events := decoder.Feed(pending.prefix)
	events = append(events, decoder.Finish()...)
	for _, event := range events {
		if advice, ok := structuredStreamRetryAdvice(event, now); ok {
			return advice, true
		}
	}
	return 0, false
}

// retryBoundTurnStateOverload only handles an uncommitted HTTP Responses
// overload pinned to an existing turn-state owner. An ordinary fresh
// request still uses profile rotation; previous_response_id continues to
// use its established full-history recovery path.
func (router *Router) retryBoundTurnStateOverload(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	keys *affinityKeys,
	response *proxymodel.Response,
	outcome responseOutcome,
	pending *pendingResponse,
	started time.Time,
) (*proxymodel.Response, responseOutcome, *pendingResponse, error) {
	if request.WebSocketMessage || request.QuotaSelection.RouteKind != quotamodel.RouteKindResponses ||
		keys == nil || keys.turn == "" || keys.previous != "" ||
		!keys.hasHardAffinity(request.QuotaSelection) {
		// The SSE precommit classifier already honors original stream:true
		// when MIME is absent, and an explicit event-stream MIME regardless
		// of request.stream. Do not require the redundant request flag here.
		return response, outcome, pending, nil
	}
	for retries := 0; ; retries++ {
		if response == nil || response.StatusCode != http.StatusOK ||
			response.FirstEventCommitted || outcome.kind != responseRetry ||
			!outcome.transient || !outcome.firstEventRetry || pending == nil ||
			!pendingIsServerOverloaded(pending) {
			break
		}
		advice, present := boundOverloadAdvice(response, pending, router.now())
		delay, allow := boundOverloadRetryDelay(true, false, false, retries, time.Since(started),
			advice, present, request.RequestID)
		if !allow {
			break
		}
		// Classification has already released this failed stream's
		// admission permit. Preserve its error bytes until a subsequent
		// owner-specific upstream attempt has been classified.
		releaseProfileInflight(response.Body)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			closePendingResponse(pending)
			return nil, responseOutcome{}, nil, ctx.Err()
		case <-timer.C:
		}
		next, err := router.executeWithProfileInflightWait(ctx, request, account, true)
		if err != nil {
			closePendingResponse(pending)
			return nil, responseOutcome{}, nil, err
		}
		if next == nil || next.Body == nil {
			closePendingResponse(pending)
			return nil, responseOutcome{}, nil, errors.New("owner precommit retry lost upstream body")
		}
		nextOutcome, nextPending, classifyErr := router.classify(next, account.Provider.Kind)
		if classifyErr != nil {
			closePendingResponse(nextPending)
			closePendingResponse(pending)
			return nil, responseOutcome{}, nil, classifyErr
		}
		closePendingResponse(pending)
		response, outcome, pending = next, nextOutcome, nextPending
	}
	return response, outcome, pending, nil
}

// The extra Prodex 0.437.1 retry is for an actual server_is_overloaded SSE
// precommit failure, not arbitrary rate limits, quota failures or errors
// after visible output. Only an event with the correct type and nested
// server_is_overloaded code qualifies.
func pendingIsServerOverloaded(pending *pendingResponse) bool {
	if pending == nil || len(pending.prefix) == 0 {
		return false
	}
	decoder := sse.NewDecoder(len(pending.prefix))
	events := decoder.Feed(pending.prefix)
	events = append(events, decoder.Finish()...)
	for _, data := range events {
		var event struct {
			Type     string `json:"type"`
			Response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			} `json:"response"`
		}
		if json.Unmarshal(data, &event) == nil &&
			event.Type == "response.failed" &&
			event.Response.Error.Code == "server_is_overloaded" {
			return true
		}
	}
	return false
}
