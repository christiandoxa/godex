package routing

import (
	"context"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) tryFreshCandidates(
	ctx context.Context,
	request proxymodel.Request,
	candidates []proxymodel.Account,
	last **pendingResponse,
	excluded, retryable map[string]bool,
	firstEventRetryUsed *bool,
) (proxymodel.Forwarded, bool, bool, bool, error) {
	sawTransient := false
	sawSaturated := false
	for _, account := range candidates {
		allowed, err := router.reserveRouteCircuitProbe(ctx, account.ID, request.QuotaSelection, router.now())
		if err != nil {
			return proxymodel.Forwarded{}, false, sawTransient, sawSaturated, err
		}
		if !allowed {
			sawTransient = true
			continue
		}
		request.FirstEventRetryUsed = *firstEventRetryUsed
		result, found, pending, saturated, err := router.tryFreshCandidate(ctx, request, candidates, account)
		if err != nil || found {
			return result, found, sawTransient, sawSaturated, err
		}
		if saturated {
			sawSaturated = true
			continue
		}
		excluded[account.ID] = true
		if pending != nil {
			if pending.firstEventRetry {
				*firstEventRetryUsed = true
			}
			if pending.authFailure || pending.quota {
				retryable[account.ID] = false
			}
			sawTransient = sawTransient || pending.transient
			replacePending(last, pending)
			if pending.quota && request.QuotaSelection.RouteKind == quotamodel.RouteKindCompact &&
				compactQuotaFallbackExhausted(candidates, account.ID, excluded) {
				result, err := finishFresh(last)
				return result, true, sawTransient, sawSaturated, err
			}
		}
	}
	return proxymodel.Forwarded{}, false, sawTransient, sawSaturated, nil
}

func (router *Router) tryFreshCandidate(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
) (proxymodel.Forwarded, bool, *pendingResponse, bool, error) {
	result, pending, saturated, err := router.freshAttempt(ctx, request, account)
	if err != nil {
		return proxymodel.Forwarded{}, false, nil, false, err
	}
	if saturated {
		return proxymodel.Forwarded{}, false, nil, true, nil
	}
	if result != nil {
		return *result, true, nil, false, nil
	}
	if pending == nil || !router.autoRedeem || !router.quotaBlockedAccount(account.ID) ||
		!freshAutoRedeemAllowedForResponse(request, pending) {
		return proxymodel.Forwarded{}, false, pending, false, nil
	}
	if pending.firstEventRetry {
		request.FirstEventRetryUsed = true
	}
	redeemResult, found, nextPending, err := router.tryFreshQuotaRedeem(ctx, request, accounts, account, pending)
	return redeemResult, found, nextPending, false, err
}

func (router *Router) tryFreshQuotaRedeem(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
	pending *pendingResponse,
) (proxymodel.Forwarded, bool, *pendingResponse, error) {
	redeemed, ok, err := router.tryAutoRedeem(ctx, accounts, account.ID, request)
	if err != nil {
		pending.close()
		return proxymodel.Forwarded{}, false, nil, err
	}
	if !ok {
		return proxymodel.Forwarded{}, false, pending, nil
	}
	pending.close()
	for {
		result, retryPending, saturated, err := router.freshAttempt(ctx, request, redeemed)
		if err != nil {
			if ctx.Err() != nil {
				return proxymodel.Forwarded{}, false, nil, ctx.Err()
			}
			return proxymodel.Forwarded{}, false, nil, nil
		}
		if saturated {
			if err := router.waitForProfileInflight(ctx); err != nil {
				return proxymodel.Forwarded{}, false, nil, err
			}
			continue
		}
		if result != nil {
			return *result, true, nil, nil
		}
		return proxymodel.Forwarded{}, false, retryPending, nil
	}
}

func freshAutoRedeemPool(
	accounts []proxymodel.Account,
	excluded map[string]bool,
) []proxymodel.Account {
	if len(excluded) == 0 {
		return accounts
	}
	result := make([]proxymodel.Account, 0, len(accounts))
	for _, account := range accounts {
		if !excluded[account.ID] {
			result = append(result, account)
		}
	}
	return result
}

func (router *Router) tryFreshAutoRedeem(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
) (proxymodel.Forwarded, bool, error) {
	accounts = router.previousResponseFailureAccounts(accounts, request, router.now())
	if len(accounts) == 0 {
		return proxymodel.Forwarded{}, false, nil
	}
	redeemed, ok, err := router.tryAutoRedeem(ctx, accounts, "", request)
	if err != nil || !ok {
		return proxymodel.Forwarded{}, false, err
	}
	allowed, err := router.reserveRouteCircuitProbe(ctx, redeemed.ID, request.QuotaSelection, router.now())
	if err != nil || !allowed {
		return proxymodel.Forwarded{}, false, err
	}
	result, redeemErr := router.redeemedAttempt(ctx, request, redeemed)
	if redeemErr != nil {
		return proxymodel.Forwarded{}, false, redeemErr
	}
	return result, true, nil
}

func (router *Router) freshAttempt(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Forwarded, *pendingResponse, bool, error) {
	response, acquired, err := router.tryExecuteWithProfileInflight(ctx, request, account, false)
	if !acquired {
		return nil, nil, true, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, false, ctx.Err()
		}
		router.recordRouteFailure(ctx, account.ID, request.QuotaSelection)
		if isTransportFailure(err) {
			router.persistTransportBackoff(ctx, account.ID, request.QuotaSelection)
		}
		return nil, &pendingResponse{accountID: account.ID, transient: true}, false, nil
	}
	outcome, pending, err := router.classify(response, account.Provider.Kind)
	if err != nil {
		if ctx.Err() != nil {
			if pending != nil {
				pending.close()
			}
			return nil, nil, false, ctx.Err()
		}
		if pending != nil && pending.transient {
			router.recordRouteFailure(ctx, account.ID, request.QuotaSelection)
			if isTransportFailure(err) {
				router.persistTransportBackoff(ctx, account.ID, request.QuotaSelection)
			}
			pending.close()
			return nil, &pendingResponse{accountID: account.ID, transient: true}, false, nil
		}
		if pending != nil {
			pending.close()
		}
		return nil, nil, false, &proxymodel.Error{StatusCode: 502, Message: "upstream response failed before commitment"}
	}
	if outcome.kind == responsePass {
		router.clearQuotaBlocked(account.ID)
		router.recordRouteOutcome(ctx, account.ID, request.QuotaSelection, response, outcome)
		result := &proxymodel.Forwarded{Response: response, Prefix: pending.prefix, AccountID: account.ID, Failed: outcome.failed}
		return result, nil, false, nil
	}
	router.recordRouteOutcome(ctx, account.ID, request.QuotaSelection, response, outcome)
	router.applyRetryOutcome(ctx, account.ID, request.QuotaSelection, outcome)
	pending.firstEventRetry = outcome.firstEventRetry
	pending.accountID = account.ID
	pending.authFailure = outcome.kind == responseAuthFailure
	pending.quota = outcome.quota
	pending.transient = outcome.transient
	return nil, pending, false, nil
}

func (router *Router) applyRetryOutcome(ctx context.Context, accountID string, selection quotamodel.Selection, outcome responseOutcome) {
	if outcome.quota {
		router.markQuotaBlocked(accountID)
		router.cacheQuotaFailure(accountID, selection, outcome.quarantine)
	} else {
		router.clearQuotaBlocked(accountID)
	}
	if outcome.kind == responseAuthFailure {
		router.quarantineAuthFailure(accountID, 60*time.Second)
		return
	}
	if outcome.kind == responseRetry && !outcome.transport {
		duration := outcome.quarantine
		if duration <= 0 {
			duration = defaultProfileRetryBackoff
		}
		router.persistRetryBackoff(ctx, accountID, duration)
	}
}

func replacePending(last **pendingResponse, pending *pendingResponse) {
	closePending(last)
	*last = pending
}

func closePending(last **pendingResponse) {
	if *last != nil {
		(*last).close()
		*last = nil
	}
}

func finishFresh(last **pendingResponse) (proxymodel.Forwarded, error) {
	if *last == nil {
		return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 502, Message: "all eligible accounts failed before upstream response commitment"}
	}
	pending := *last
	*last = nil
	if pending.response == nil {
		return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 502, Message: "all eligible accounts failed before upstream response commitment"}
	}
	return proxymodel.Forwarded{Response: pending.response, Prefix: pending.prefix, AccountID: pending.accountID, Failed: true}, nil
}
