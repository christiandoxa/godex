package routing

import (
	"context"
	"net/http"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (router *Router) forwardFresh(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
) (proxymodel.Forwarded, error) {
	candidates := router.candidates(accounts, router.now())
	if len(candidates) == 0 {
		if account, ok := router.websocketQuotaReplayLastChance(request, accounts); ok {
			return router.forwardWebSocketQuotaLastChance(ctx, request, account)
		}
		return router.forwardFreshWithoutCandidates(ctx, request, accounts)
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
		current := freshRetryCandidates(router, candidates, retryable, router.now())
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
			delay, ok := router.freshRecoveryDelay(candidates, retryable, request.RequestID, recoverySweeps)
			if !ok {
				break
			}
			if err := router.waitForFreshRecovery(ctx, delay); err != nil {
				return proxymodel.Forwarded{}, err
			}
			recoverySweeps++
			refreshedAccounts, refreshedCandidates, err := router.reloadFreshCandidatesAfterRecoveryWait(ctx, candidates, retryable)
			if err != nil {
				return proxymodel.Forwarded{}, err
			}
			accounts, candidates = refreshedAccounts, refreshedCandidates
			continue
		}
		result, found, transient, saturated, err := router.tryFreshCandidates(
			ctx, request, current, &last, excluded, retryable, &firstEventRetryUsed,
		)
		if err != nil || found {
			return result, err
		}
		if saturated {
			if err := router.waitForProfileInflight(ctx); err != nil {
				return proxymodel.Forwarded{}, err
			}
			refreshed, loadErr := router.loadAccounts(ctx)
			if loadErr != nil {
				return proxymodel.Forwarded{}, loadErr
			}
			accounts = refreshed
			candidates = router.candidates(accounts, router.now())
			for _, account := range candidates {
				if _, exists := retryable[account.ID]; !exists {
					retryable[account.ID] = true
				}
			}
			if len(candidates) == 0 && last == nil {
				return router.forwardFreshWithoutCandidates(ctx, request, accounts)
			}
			continue
		}
		if !transient || !hasRetryableFreshAccount(retryable) {
			break
		}
		delay, ok := router.freshRecoveryDelay(candidates, retryable, request.RequestID, recoverySweeps)
		if !ok {
			break
		}
		if err := router.waitForFreshRecovery(ctx, delay); err != nil {
			return proxymodel.Forwarded{}, err
		}
		recoverySweeps++
		refreshedAccounts, refreshedCandidates, err := router.reloadFreshCandidatesAfterRecoveryWait(ctx, candidates, retryable)
		if err != nil {
			return proxymodel.Forwarded{}, err
		}
		accounts, candidates = refreshedAccounts, refreshedCandidates
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

func freshRetryCandidates(
	router *Router,
	candidates []proxymodel.Account,
	retryable map[string]bool,
	now time.Time,
) []proxymodel.Account {
	result := make([]proxymodel.Account, 0, len(candidates))
	for _, account := range candidates {
		if retryable[account.ID] && account.Enabled && !account.EligibleAfter.After(now) &&
			!router.isQuarantined(account.ID, now) {
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
