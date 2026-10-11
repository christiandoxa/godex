package routing

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// A full-context replay may wait for a quota-positive account. Local admission
// and transport cooldown are temporary; quota exhaustion, auth failures, and
// an existing retry quarantine remain terminal for this recovery handoff.
func (router *Router) websocketQuotaFullContextFallbackRecoverable(
	request proxymodel.Request,
	accounts []proxymodel.Account,
	failed proxymodel.Account,
) bool {
	now := router.now()
	for _, candidate := range accounts {
		if candidate.ID == failed.ID || candidate.ID == "" || candidate.Home == "" ||
			!candidate.Enabled || candidate.EligibleAfter.After(now) ||
			!sameWebSocketProvider(failed, candidate) ||
			router.authFailureQuarantined(candidate.ID, now) ||
			router.isQuarantined(candidate.ID, now) ||
			router.quotaBlockedAccount(candidate.ID) {
			continue
		}
		if state, cached := router.cachedQuotaCheck(candidate.ID, request.QuotaSelection, now); cached &&
			!state.ready && (state.retryAt.IsZero() || state.retryAt.After(now)) {
			continue
		}
		return true
	}
	return false
}

func (router *Router) websocketQuotaReplayLastChance(
	request proxymodel.Request,
	accounts []proxymodel.Account,
) (proxymodel.Account, bool) {
	if !request.WebSocketMessage {
		return proxymodel.Account{}, false
	}
	metadata := parseWebSocketRequestMetadata(request)
	if metadata.sessionID == "" || metadata.previousResponseID != "" || metadata.turnState != "" {
		return proxymodel.Account{}, false
	}
	now := router.now()
	for _, blocked := range accounts {
		if blocked.ID == "" || blocked.Home == "" || !blocked.Enabled {
			continue
		}
		if !blocked.EligibleAfter.After(now) && !router.quotaBlockedAccount(blocked.ID) {
			continue
		}
		decision := router.websocketQuotaFallbackDecisionWithContext(
			request, accounts, blocked, false,
		)
		if decision.kind == websocketQuotaFallbackLastChance {
			return decision.account, true
		}
	}
	return proxymodel.Account{}, false
}
