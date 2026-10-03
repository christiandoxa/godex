package routing

import (
	"context"
	"net/http"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (router *Router) forwardBound(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	owner string,
) (proxymodel.Forwarded, error) {
	account, err := boundOwnerAccount(accounts, owner)
	if err != nil {
		return proxymodel.Forwarded{}, err
	}
	account, err = router.prepareBoundOwner(ctx, request, accounts, account)
	if err != nil {
		return proxymodel.Forwarded{}, err
	}
	response, err := router.execute(ctx, request, account)
	if err != nil {
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "conversation owner could not be reached; continuity was preserved",
		}
	}
	if !router.autoRedeem {
		return router.legacyBoundResponse(account, response), nil
	}
	return router.handleBoundResponse(ctx, request, accounts, account, response)
}

func boundOwnerAccount(accounts []proxymodel.Account, owner string) (proxymodel.Account, error) {
	for _, account := range accounts {
		if account.ID == owner {
			return account, nil
		}
	}
	return proxymodel.Account{}, &proxymodel.Error{
		StatusCode: http.StatusConflict,
		Message:    "conversation owner is no longer registered; continuity was preserved",
	}
}

func (router *Router) prepareBoundOwner(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
) (proxymodel.Account, error) {
	if !account.Enabled {
		return proxymodel.Account{}, boundOwnerUnavailable()
	}
	if router.autoRedeem && router.boundOwnerBlocked(account) {
		redeemed, ok, err := router.tryAutoRedeem(ctx, accounts, account.ID, request)
		if err != nil {
			return proxymodel.Account{}, err
		}
		if ok {
			account = redeemed
		}
	}
	if router.boundOwnerBlocked(account) {
		return proxymodel.Account{}, boundOwnerUnavailable()
	}
	return account, nil
}

func (router *Router) boundOwnerBlocked(account proxymodel.Account) bool {
	now := router.now()
	return router.isQuarantined(account.ID, now) || account.EligibleAfter.After(now)
}

func boundOwnerUnavailable() error {
	return &proxymodel.Error{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "conversation owner is temporarily unavailable; continuity was preserved",
	}
}

func (router *Router) legacyBoundResponse(
	account proxymodel.Account,
	response *proxymodel.Response,
) proxymodel.Forwarded {
	if response.StatusCode == http.StatusUnauthorized {
		router.quarantineAccount(account.ID, time.Minute)
	}
	return proxymodel.Forwarded{Response: response, AccountID: account.ID}
}

func (router *Router) handleBoundResponse(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
	response *proxymodel.Response,
) (proxymodel.Forwarded, error) {
	outcome, pending, err := router.classify(response, account.Provider.Kind)
	if err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "conversation owner response failed before commitment",
		}
	}
	if outcome.kind == responsePass {
		router.clearQuotaBlocked(account.ID)
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	router.applyRetryOutcome(account.ID, outcome)
	if !outcome.quota {
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	return router.tryBoundRedeemedRetry(ctx, request, accounts, account.ID, outcome, pending)
}

func (router *Router) tryBoundRedeemedRetry(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	owner string,
	outcome responseOutcome,
	pending *pendingResponse,
) (proxymodel.Forwarded, error) {
	redeemed, ok, err := router.tryAutoRedeem(ctx, accounts, owner, request)
	if err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, err
	}
	if !ok {
		return pendingForwarded(owner, outcome, pending), nil
	}
	closePendingResponse(pending)
	return router.redeemedAttempt(ctx, request, redeemed)
}

func pendingForwarded(
	accountID string,
	outcome responseOutcome,
	pending *pendingResponse,
) proxymodel.Forwarded {
	return proxymodel.Forwarded{
		Response: pending.response, Prefix: pending.prefix, AccountID: accountID,
		Failed: outcome.failed || outcome.kind != responsePass,
	}
}

func closePendingResponse(pending *pendingResponse) {
	if pending != nil {
		pending.close()
	}
}

func (router *Router) forwardFresh(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
) (proxymodel.Forwarded, error) {
	candidates := router.candidates(accounts, router.now())
	if len(candidates) == 0 {
		return router.forwardFreshWithoutCandidates(ctx, request, accounts)
	}
	var last *pendingResponse
	defer closePending(&last)
	excluded := make(map[string]bool, len(candidates))
	result, found, err := router.tryFreshCandidates(ctx, request, candidates, &last, excluded)
	if err != nil || found {
		return result, err
	}
	remaining := freshAutoRedeemPool(accounts, excluded)
	if redeemed, ok, redeemErr := router.tryFreshAutoRedeem(ctx, request, remaining); ok || redeemErr != nil {
		if redeemErr == nil {
			return redeemed, nil
		}
		if ctx.Err() != nil {
			return proxymodel.Forwarded{}, ctx.Err()
		}
	}
	return finishFresh(&last)
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

func (router *Router) tryFreshCandidates(
	ctx context.Context,
	request proxymodel.Request,
	candidates []proxymodel.Account,
	last **pendingResponse,
	excluded map[string]bool,
) (proxymodel.Forwarded, bool, error) {
	for index, account := range candidates {
		result, found, pending, err := router.tryFreshCandidate(ctx, request, candidates, account)
		if err != nil || found {
			return result, found, err
		}
		excluded[account.ID] = true
		if pending != nil {
			if pending.firstEventRetry && index+1 < len(candidates) {
				request.FirstEventRetryUsed = true
			}
			replacePending(last, pending)
		}
	}
	return proxymodel.Forwarded{}, false, nil
}

func (router *Router) tryFreshCandidate(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
) (proxymodel.Forwarded, bool, *pendingResponse, error) {
	result, pending, err := router.freshAttempt(ctx, request, account)
	if err != nil {
		return proxymodel.Forwarded{}, false, nil, err
	}
	if result != nil {
		return *result, true, nil, nil
	}
	if pending == nil || !router.autoRedeem || !router.quotaBlockedAccount(account.ID) {
		return proxymodel.Forwarded{}, false, pending, nil
	}
	if pending.firstEventRetry {
		request.FirstEventRetryUsed = true
	}
	return router.tryFreshQuotaRedeem(ctx, request, accounts, account, pending)
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
	result, retryPending, err := router.freshAttempt(ctx, request, redeemed)
	if err != nil {
		if ctx.Err() != nil {
			return proxymodel.Forwarded{}, false, nil, ctx.Err()
		}
		return proxymodel.Forwarded{}, false, nil, nil
	}
	if result != nil {
		return *result, true, nil, nil
	}
	return proxymodel.Forwarded{}, false, retryPending, nil
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
	redeemed, ok, err := router.tryAutoRedeem(ctx, accounts, "", request)
	if err != nil || !ok {
		return proxymodel.Forwarded{}, false, err
	}
	result, redeemErr := router.redeemedAttempt(ctx, request, redeemed)
	if redeemErr != nil {
		return proxymodel.Forwarded{}, false, redeemErr
	}
	return result, true, nil
}

func (router *Router) freshAttempt(ctx context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Forwarded, *pendingResponse, error) {
	response, err := router.execute(ctx, request, account)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, nil
	}
	outcome, pending, err := router.classify(response, account.Provider.Kind)
	if err != nil {
		if pending != nil {
			pending.close()
		}
		return nil, nil, &proxymodel.Error{StatusCode: 502, Message: "upstream response failed before commitment"}
	}
	if outcome.kind == responsePass {
		router.clearQuotaBlocked(account.ID)
		result := &proxymodel.Forwarded{Response: response, Prefix: pending.prefix, AccountID: account.ID, Failed: outcome.failed}
		return result, nil, nil
	}
	router.applyRetryOutcome(account.ID, outcome)
	pending.firstEventRetry = outcome.firstEventRetry
	pending.accountID = account.ID
	return nil, pending, nil
}

func (router *Router) applyRetryOutcome(accountID string, outcome responseOutcome) {
	if outcome.quota {
		router.markQuotaBlocked(accountID)
	} else {
		router.clearQuotaBlocked(accountID)
	}
	if outcome.kind == responseAuthFailure {
		router.quarantineAccount(accountID, 60e9)
		return
	}
	if outcome.quarantine > 0 {
		router.quarantineAccount(accountID, outcome.quarantine)
	}
}

func replacePending(last **pendingResponse, pending *pendingResponse) {
	closePending(last)
	*last = pending
}

func closePending(pending **pendingResponse) {
	if *pending != nil {
		(*pending).close()
		*pending = nil
	}
}

func finishFresh(last **pendingResponse) (proxymodel.Forwarded, error) {
	if *last == nil {
		return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 502, Message: "all eligible accounts failed before upstream response commitment"}
	}
	pending := *last
	*last = nil
	return proxymodel.Forwarded{Response: pending.response, Prefix: pending.prefix, AccountID: pending.accountID, Failed: true}, nil
}
