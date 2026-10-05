package routing

import (
	"context"
	"net/http"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) forwardFresh(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
) (proxymodel.Forwarded, error) {
	candidates := router.requestCandidatesForRequest(accounts, request, router.now())
	quotaRefreshAttempted := false
	if len(candidates) == 0 {
		refreshed, checked, err := router.refreshQuotaExcluded(ctx, accounts, request.QuotaSelection, router.now())
		if err != nil {
			return proxymodel.Forwarded{}, err
		}
		quotaRefreshAttempted = checked
		if checked {
			accounts = refreshed
			candidates = router.requestCandidatesForRequest(accounts, request, router.now())
		}
		if len(candidates) == 0 {
			if account, ok := router.websocketQuotaReplayLastChance(request, accounts); ok {
				return router.forwardWebSocketQuotaLastChance(ctx, request, account)
			}
			return router.forwardFreshWithoutCandidates(ctx, request, accounts)
		}
	}

	var last *pendingResponse
	defer closePending(&last)
	excluded := make(map[string]bool, len(candidates))
	retryable := make(map[string]bool, len(candidates))
	for _, account := range candidates {
		retryable[account.ID] = true
	}
	firstEventRetryUsed := request.FirstEventRetryUsed
	autoRedeemAttempted := false

	for recoverySweeps := 0; ; {
		request.FirstEventRetryUsed = firstEventRetryUsed
		current := freshRetryCandidates(router, candidates, retryable, request.QuotaSelection, router.now())
		if len(current) == 0 {
			if lastChance, ok := router.websocketQuotaReplayLastChance(request, accounts); ok {
				return router.forwardWebSocketQuotaLastChance(ctx, request, lastChance)
			}
			if recoverySweeps > 0 && !autoRedeemAttempted {
				autoRedeemAttempted = true
				remaining := freshAutoRedeemPool(accounts, excluded)
				if redeemed, ok, redeemErr := router.tryFreshAutoRedeem(ctx, request, remaining); ok || redeemErr != nil {
					if redeemErr == nil {
						return redeemed, nil
					}
					if ctx.Err() != nil {
						return proxymodel.Forwarded{}, ctx.Err()
					}
				}
			}
			if !hasRetryableFreshAccount(retryable) {
				break
			}
			delay, ok := router.freshRecoveryDelay(candidates, retryable, request.QuotaSelection, request.RequestID, recoverySweeps)
			if !ok {
				break
			}
			if err := router.waitForFreshRecovery(ctx, delay); err != nil {
				return proxymodel.Forwarded{}, err
			}
			recoverySweeps++
			freshAccounts, freshCandidates, _, err := router.refreshFreshCandidatesAfterWait(ctx, request, retryable)
			if err != nil {
				return proxymodel.Forwarded{}, err
			}
			accounts, candidates = freshAccounts, freshCandidates
			continue
		}

		result, found, transient, saturated, err := router.tryFreshCandidates(
			ctx, request, current, &last, excluded, retryable, &firstEventRetryUsed,
		)
		if err != nil || found {
			return result, err
		}

		if !quotaRefreshAttempted && router.quota != nil {
			refreshed, checked, refreshErr := router.refreshQuotaExcluded(ctx, accounts, request.QuotaSelection, router.now())
			if refreshErr != nil {
				return proxymodel.Forwarded{}, refreshErr
			}
			quotaRefreshAttempted = checked
			if checked {
				accounts = refreshed
				available := router.requestCandidatesForRequestWithoutRotation(accounts, request, router.now())
				newCandidate := false
				for _, account := range available {
					if _, exists := retryable[account.ID]; !exists {
						retryable[account.ID] = true
						newCandidate = true
					}
				}
				if newCandidate {
					candidates = available
					continue
				}
			}
		}

		if saturated {
			if err := router.waitForProfileInflight(ctx); err != nil {
				return proxymodel.Forwarded{}, err
			}
			recoverySweeps++
			freshAccounts, freshCandidates, _, err := router.refreshFreshCandidatesAfterWait(ctx, request, retryable)
			if err != nil {
				return proxymodel.Forwarded{}, err
			}
			accounts, candidates = freshAccounts, freshCandidates
			continue
		}

		if transient && hasRetryableFreshAccount(retryable) {
			delay, ok := router.freshRecoveryDelay(candidates, retryable, request.QuotaSelection, request.RequestID, recoverySweeps)
			if !ok {
				break
			}
			if err := router.waitForFreshRecovery(ctx, delay); err != nil {
				return proxymodel.Forwarded{}, err
			}
			recoverySweeps++
			freshAccounts, freshCandidates, _, err := router.refreshFreshCandidatesAfterWait(ctx, request, retryable)
			if err != nil {
				return proxymodel.Forwarded{}, err
			}
			accounts, candidates = freshAccounts, freshCandidates
			continue
		}
		break
	}

	remaining := freshAutoRedeemPool(accounts, excluded)
	if !autoRedeemAttempted {
		if redeemed, ok, redeemErr := router.tryFreshAutoRedeem(ctx, request, remaining); ok || redeemErr != nil {
			if redeemErr == nil {
				return redeemed, nil
			}
			if ctx.Err() != nil {
				return proxymodel.Forwarded{}, ctx.Err()
			}
		}
	}
	return finishFresh(&last)
}

func (router *Router) refreshFreshCandidatesAfterWait(
	ctx context.Context,
	request proxymodel.Request,
	retryable map[string]bool,
) ([]proxymodel.Account, []proxymodel.Account, []proxymodel.Account, error) {
	accounts, err := router.loadAccounts(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	accounts, _, err = router.refreshQuotaExcluded(ctx, accounts, request.QuotaSelection, router.now())
	if err != nil {
		return nil, nil, nil, err
	}
	candidates := router.requestCandidatesForRequestWithoutRotation(accounts, request, router.now())
	available := make(map[string]bool, len(candidates))
	for _, account := range candidates {
		available[account.ID] = true
	}
	for id, canRetry := range retryable {
		if canRetry && !available[id] {
			delete(retryable, id)
		}
	}
	for _, account := range candidates {
		if _, exists := retryable[account.ID]; !exists {
			retryable[account.ID] = true
		}
	}
	current := freshRetryCandidates(router, candidates, retryable, request.QuotaSelection, router.now())
	return accounts, candidates, current, nil
}

func freshRetryCandidates(
	router *Router,
	candidates []proxymodel.Account,
	retryable map[string]bool,
	selection quotamodel.Selection,
	now time.Time,
) []proxymodel.Account {
	result := make([]proxymodel.Account, 0, len(candidates))
	for _, account := range candidates {
		if retryable[account.ID] && account.Enabled && !account.EligibleAfter.After(now) &&
			!router.isQuarantined(account.ID, now) && router.transportBackoffRemaining(account.ID, selection, now) == 0 &&
			router.routeCircuitRemaining(account.ID, selection, now) == 0 {
			result = append(result, account)
		}
	}
	return result
}

func hasRetryableFreshAccount(retryable map[string]bool) bool {
	for _, canRetry := range retryable {
		if canRetry {
			return true
		}
	}
	return false
}

func (router *Router) forwardFreshWithoutCandidates(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
) (proxymodel.Forwarded, error) {
	if redeemed, ok, err := router.tryFreshAutoRedeem(ctx, request, accounts); err != nil {
		return proxymodel.Forwarded{}, err
	} else if ok {
		return redeemed, nil
	}
	return proxymodel.Forwarded{}, &proxymodel.Error{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "no enabled account is available",
	}
}
