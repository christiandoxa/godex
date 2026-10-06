//go:build !linux

package superexpose

type appServerActivity struct {
	active       bool
	activeTurnID string
}

func appServerThreadActivity(resolvedSessionTarget, bool) (*appServerActivity, bool, error) {
	return nil, false, sessionQueueUnsupported
}

func appServerQueueAddOnce(resolvedSessionTarget, string) queueInvocation {
	return queueInvocation{outcome: queuePreflight}
}

func appServerPreempt(resolvedSessionTarget) (queuePreemptResult, error) {
	return queuePreemptResult{}, sessionQueueUnsupported
}
