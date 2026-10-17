package routing

import "time"

// RequestWatchdogBudget is the shared precommit budget used by delivery
// layers to cover admission, queueing, and routing as one request lifecycle.
func (router *Router) RequestWatchdogBudget() time.Duration {
	if router == nil || router.precommitWatchdogBudget <= 0 {
		return 5 * time.Minute
	}
	return router.precommitWatchdogBudget
}
