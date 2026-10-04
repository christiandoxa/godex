package routing

import "time"

var websocketPreviousResponseRetryDelays = [...]time.Duration{
	75 * time.Millisecond,
	200 * time.Millisecond,
	500 * time.Millisecond,
}

type websocketPreviousResponsePlan struct {
	retryDelay                    time.Duration
	retryOwner                    bool
	staleContinuation             bool
	requestRequiresLockedAffinity bool
	freshBlockedWithoutAffinity   bool
}

type websocketPreviousResponsePlanInput struct {
	previousPresent                 bool
	hasTurnStateRetry               bool
	requestRequiresPreviousAffinity bool
	trustedPreviousAffinity         bool
	requestTurnStatePresent         bool
	retryIndex                      int
}

func planWebSocketPreviousResponse(input websocketPreviousResponsePlanInput) websocketPreviousResponsePlan {
	websocketRequiresAffinity := input.trustedPreviousAffinity &&
		input.previousPresent &&
		!input.requestTurnStatePresent
	requestRequiresLockedAffinity := input.requestRequiresPreviousAffinity || websocketRequiresAffinity
	lockedAffinityRetry := input.requestRequiresPreviousAffinity && !input.hasTurnStateRetry

	retryReason := input.hasTurnStateRetry || lockedAffinityRetry
	if retryReason && input.retryIndex >= 0 && input.retryIndex < len(websocketPreviousResponseRetryDelays) {
		return websocketPreviousResponsePlan{
			retryDelay:                    websocketPreviousResponseRetryDelays[input.retryIndex],
			retryOwner:                    true,
			requestRequiresLockedAffinity: requestRequiresLockedAffinity,
		}
	}

	freshFailClosed := input.previousPresent || requestRequiresLockedAffinity
	return websocketPreviousResponsePlan{
		staleContinuation:             input.previousPresent,
		requestRequiresLockedAffinity: requestRequiresLockedAffinity,
		freshBlockedWithoutAffinity: freshFailClosed &&
			!input.hasTurnStateRetry &&
			!requestRequiresLockedAffinity,
	}
}
