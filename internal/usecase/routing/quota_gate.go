package routing

import (
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) requestCandidates(
	accounts []proxymodel.Account,
	selection quotamodel.Selection,
	now time.Time,
) []proxymodel.Account {
	return router.requestCandidatesMode(accounts, selection, now, true)
}

func (router *Router) requestCandidatesWithoutRotation(
	accounts []proxymodel.Account,
	selection quotamodel.Selection,
	now time.Time,
) []proxymodel.Account {
	return router.requestCandidatesMode(accounts, selection, now, false)
}

func (router *Router) requestCandidatesMode(
	accounts []proxymodel.Account,
	selection quotamodel.Selection,
	now time.Time,
	consumeRotation bool,
) []proxymodel.Account {
	return router.requestCandidatesModeWithRank(accounts, selection, now, consumeRotation, candidateRankContext{})
}

func (router *Router) requestCandidatesModeWithRank(
	accounts []proxymodel.Account,
	selection quotamodel.Selection,
	now time.Time,
	consumeRotation bool,
	rank candidateRankContext,
) []proxymodel.Account {
	router.refreshCachedQuotaChecks(accounts, selection, now)
	candidates := router.candidates(accounts, now)
	order := func(values []proxymodel.Account) []proxymodel.Account {
		return router.orderCandidatesModeWithRank(values, selection, now, consumeRotation, rank)
	}
	if len(candidates) < 2 {
		return order(candidates)
	}

	blocked := make([]bool, len(candidates))
	availableAlternative := false
	for index, account := range candidates {
		state, cached := router.cachedQuotaCheck(account.ID, selection, now)
		blocked[index] = cached && !state.ready && (state.retryAt.IsZero() || state.retryAt.After(now))
		if !blocked[index] {
			availableAlternative = true
		}
	}
	if !availableAlternative {
		return order(candidates)
	}

	filtered := make([]proxymodel.Account, 0, len(candidates))
	for index, account := range candidates {
		if !blocked[index] {
			filtered = append(filtered, account)
		}
	}
	return order(filtered)
}

func (router *Router) refreshCachedQuotaChecks(
	accounts []proxymodel.Account,
	selection quotamodel.Selection,
	now time.Time,
) {
	cached, ok := router.quota.(cachedQuotaPreflight)
	if !ok {
		return
	}
	for _, account := range accounts {
		if account.ID == "" || !account.Enabled || externalProviderKind(account.Provider.Kind) {
			continue
		}
		if _, fresh := router.cachedQuotaCheck(account.ID, selection, now); fresh {
			continue
		}
		availability, ok := cached.CachedAvailabilityForRoute(
			accountentity.Account{ID: account.ID, Enabled: account.Enabled}, selection, now,
		)
		if !ok {
			continue
		}
		router.storeQuotaCheck(quotaCheckKey{accountID: account.ID, selection: selection}, quotaCheck{
			checkedAt: now, ready: availability.Ready, retryAt: availability.RetryAt, pressure: availability.Pressure, source: availability.Source,
		})
	}
}

func (router *Router) cachedQuotaCheck(
	accountID string,
	selection quotamodel.Selection,
	now time.Time,
) (quotaCheck, bool) {
	router.mu.Lock()
	defer router.mu.Unlock()
	state, ok := router.quotaChecks[quotaCheckKey{accountID: accountID, selection: selection}]
	return state, ok && now.Sub(state.checkedAt) >= 0 && now.Sub(state.checkedAt) < quotaCheckFreshness
}
