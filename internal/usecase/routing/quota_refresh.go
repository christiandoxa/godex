package routing

import (
	"context"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const (
	quotaCheckFreshness = 5 * time.Minute
	maxQuotaChecks      = 1024
)

type quotaCheck struct {
	checkedAt          time.Time
	retryAt            time.Time
	ready              bool
	pressure           quotamodel.Pressure
	source             quotamodel.Source
	keepLaunchDeadline bool
}

type quotaCheckKey struct {
	accountID string
	selection quotamodel.Selection
}

func (router *Router) refreshQuotaExcluded(
	ctx context.Context,
	accounts []proxymodel.Account,
	selection quotamodel.Selection,
	now time.Time,
) ([]proxymodel.Account, bool, error) {
	if router.quota == nil {
		return accounts, false, nil
	}
	updated := append([]proxymodel.Account(nil), accounts...)
	checked := false
	for index, account := range updated {
		if account.ID == "" || !account.Enabled || !account.EligibleAfter.After(now) || externalProviderKind(account.Provider.Kind) {
			continue
		}
		state, err := router.getQuotaCheck(ctx, account, selection, now)
		if err != nil {
			return nil, checked, err
		}
		checked = true
		switch {
		case state.ready || (!state.retryAt.IsZero() && !state.retryAt.After(now)):
			updated[index].EligibleAfter = time.Time{}
		case !state.retryAt.IsZero():
			if !state.keepLaunchDeadline || !account.EligibleAfter.After(state.retryAt) {
				updated[index].EligibleAfter = state.retryAt
			}
		}
	}
	return updated, checked, nil
}

func (router *Router) getQuotaCheck(
	ctx context.Context,
	account proxymodel.Account,
	selection quotamodel.Selection,
	now time.Time,
) (quotaCheck, error) {
	key := quotaCheckKey{accountID: account.ID, selection: selection}
	if state, ok := router.cachedQuotaCheck(account.ID, selection, now); ok {
		return state, nil
	}

	availability, err := router.quota.AvailabilityForRoute(
		ctx,
		accountentity.Account{ID: account.ID, Enabled: account.Enabled},
		selection,
	)
	if err != nil && ctx.Err() != nil {
		return quotaCheck{}, ctx.Err()
	}
	state := quotaCheck{
		checkedAt: now, ready: err != nil || availability.Ready,
		retryAt: availability.RetryAt, pressure: availability.Pressure, source: availability.Source,
	}
	router.storeQuotaCheck(key, state)
	return state, nil
}

func (router *Router) cacheQuotaFailure(accountID string, selection quotamodel.Selection, delay time.Duration) {
	if accountID == "" {
		return
	}
	if delay < quotaCheckFreshness {
		delay = quotaCheckFreshness
	}
	now := router.now()
	router.storeQuotaCheck(quotaCheckKey{accountID: accountID, selection: selection}, quotaCheck{
		checkedAt: now, retryAt: now.Add(delay), keepLaunchDeadline: true,
	})
}

func (router *Router) storeQuotaCheck(key quotaCheckKey, state quotaCheck) {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.quotaChecks == nil {
		router.quotaChecks = make(map[quotaCheckKey]quotaCheck)
	}
	if len(router.quotaChecks) >= maxQuotaChecks {
		var oldestKey quotaCheckKey
		var oldest time.Time
		for candidate, cached := range router.quotaChecks {
			if oldestKey.accountID == "" || cached.checkedAt.Before(oldest) {
				oldestKey, oldest = candidate, cached.checkedAt
			}
		}
		delete(router.quotaChecks, oldestKey)
	}
	router.quotaChecks[key] = state
}
