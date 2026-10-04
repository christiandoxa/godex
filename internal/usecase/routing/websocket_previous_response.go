package routing

import "time"

var websocketPreviousResponseRetryDelays = [...]time.Duration{
	75 * time.Millisecond,
	200 * time.Millisecond,
	500 * time.Millisecond,
}

type websocketPreviousResponsePlan struct {
	retryDelay        time.Duration
	retryOwner        bool
	staleContinuation bool
}

type websocketPreviousResponsePlanInput struct {
	previousPresent bool
	hasTurnState    bool
	lockedAffinity  bool
	retryIndex      int
}

func planWebSocketPreviousResponse(input websocketPreviousResponsePlanInput) websocketPreviousResponsePlan {
	if !input.previousPresent {
		return websocketPreviousResponsePlan{}
	}
	retryReason := input.hasTurnState || input.lockedAffinity
	if retryReason && input.retryIndex >= 0 && input.retryIndex < len(websocketPreviousResponseRetryDelays) {
		return websocketPreviousResponsePlan{
			retryDelay: websocketPreviousResponseRetryDelays[input.retryIndex],
			retryOwner: true,
		}
	}
	return websocketPreviousResponsePlan{staleContinuation: true}
}
