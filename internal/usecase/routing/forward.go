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
	response, err := router.executeWithProfileInflightWait(ctx, request, account, true)
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
		router.quarantineAuthFailure(account.ID, time.Minute)
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
