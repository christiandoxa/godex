package routing

import (
	"context"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (router *Router) reloadFreshCandidatesAfterRecoveryWait(
	ctx context.Context,
	previous []proxymodel.Account,
	retryable map[string]bool,
) ([]proxymodel.Account, []proxymodel.Account, error) {
	accounts, err := router.loadAccounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	available := router.recoveryAvailableAccounts(accounts, router.now())
	byID := make(map[string]proxymodel.Account, len(available))
	for _, account := range available {
		byID[account.ID] = account
	}

	candidates := make([]proxymodel.Account, 0, len(available))
	for _, account := range previous {
		refreshed, exists := byID[account.ID]
		if !exists {
			continue
		}
		candidates = append(candidates, refreshed)
		delete(byID, account.ID)
	}
	for _, account := range available {
		if _, exists := byID[account.ID]; !exists {
			continue
		}
		candidates = append(candidates, account)
		delete(byID, account.ID)
	}

	present := make(map[string]struct{}, len(candidates))
	for _, account := range candidates {
		present[account.ID] = struct{}{}
		if _, exists := retryable[account.ID]; !exists {
			retryable[account.ID] = true
		}
	}
	for accountID := range retryable {
		if _, exists := present[accountID]; !exists {
			delete(retryable, accountID)
		}
	}
	return accounts, candidates, nil
}

func (router *Router) recoveryAvailableAccounts(
	accounts []proxymodel.Account,
	now time.Time,
) []proxymodel.Account {
	available := make([]proxymodel.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.ID == "" || account.Home == "" || !account.Enabled || account.EligibleAfter.After(now) {
			continue
		}
		if router.authFailureQuarantined(account.ID, now) ||
			(router.quotaBlockedAccount(account.ID) && router.isQuarantined(account.ID, now)) {
			continue
		}
		available = append(available, account)
	}
	return available
}
