package routing

import (
	"context"
	"net/http"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) forwardBound(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	owner string,
	keys *affinityKeys,
) (proxymodel.Forwarded, error) {
	account, err := boundOwnerAccount(accounts, owner)
	if err != nil {
		return proxymodel.Forwarded{}, err
	}
	if result, handled, err := router.handleBoundWebSocketPreSendQuotaBlock(ctx, request, accounts, account); handled || err != nil {
		return result, err
	}
	account, err = router.prepareBoundOwner(ctx, request, accounts, account)
	if err != nil {
		return proxymodel.Forwarded{}, err
	}
	response, err := router.executeWithProfileInflightWait(ctx, request, account, true)
	failed, rotatePrevious := false, false
	if err == nil && !request.WebSocketMessage {
		response, failed, rotatePrevious, err = router.recoverInvalidPreviousResponse(ctx, request, account, response, keys)
	}
	if err != nil {
		if ctx.Err() != nil {
			return proxymodel.Forwarded{}, ctx.Err()
		}
		transportFailure := isTransportFailure(err)
		if transportFailure {
			router.recordTransportExecutionFailure(ctx, account.ID, request.QuotaSelection, err)
		} else {
			router.recordRouteFailure(ctx, account.ID, request.QuotaSelection)
		}
		if transportFailure {
			result, handled, recoveryErr := router.recoverBoundRetryableFailure(
				ctx, request, accounts, account, keys,
				responseOutcome{kind: responseRetry, transient: true, transport: true}, nil,
			)
			if recoveryErr != nil || handled {
				return result, recoveryErr
			}
		}
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "conversation owner could not be reached; continuity was preserved",
		}
	}
	if rotatePrevious {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		fallback := previousResponseFallbackAccounts(accounts, account.ID)
		if len(fallback) == 0 {
			return proxymodel.Forwarded{Response: staleResponsesContinuationResponse(), AccountID: account.ID, Failed: true}, nil
		}
		result, fallbackErr := router.forwardFresh(ctx, request, fallback)
		if fallbackErr != nil {
			if ctx.Err() != nil {
				return proxymodel.Forwarded{}, ctx.Err()
			}
			return proxymodel.Forwarded{Response: staleResponsesContinuationResponse(), AccountID: account.ID, Failed: true}, nil
		}
		return result, nil
	}
	if request.WebSocketMessage {
		return router.handleBoundWebSocketResponse(ctx, request, accounts, account, response)
	}
	if failed {
		return router.legacyBoundResponse(ctx, request, account, response, true), nil
	}
	return router.handleBoundResponseWithFailure(ctx, request, accounts, account, response, false, keys)
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
	affinity := requestRoutingAffinity(request)
	hardAffinity := affinity.hasAffinity()
	hardQuotaAffinity := affinity.previous != "" || affinity.turn != "" ||
		(request.QuotaSelection.RouteKind == quotamodel.RouteKindCompact && affinity.session != "")
	if !hardQuotaAffinity && account.EligibleAfter.After(router.now()) {
		refreshed, checked, err := router.refreshQuotaExcluded(ctx, []proxymodel.Account{account}, request.QuotaSelection, router.now())
		if err != nil {
			return proxymodel.Account{}, err
		}
		if checked {
			account = refreshed[0]
		}
	}
	if router.autoRedeem && !hardAffinity && router.boundOwnerBlocked(account, hardAffinity, request.QuotaSelection) {
		redeemed, ok, err := router.tryAutoRedeem(ctx, accounts, account.ID, request)
		if err != nil {
			return proxymodel.Account{}, err
		}
		if ok {
			account = redeemed
		}
	}
	if router.boundOwnerBlocked(account, hardAffinity, request.QuotaSelection) {
		return proxymodel.Account{}, boundOwnerUnavailable()
	}
	return account, nil
}

func (router *Router) boundOwnerBlocked(account proxymodel.Account, hardAffinity bool, selection quotamodel.Selection) bool {
	now := router.now()
	return router.isQuarantined(account.ID, now) ||
		(!hardAffinity && router.transportBackoffRemaining(account.ID, selection, now) > 0) ||
		(!hardAffinity && account.EligibleAfter.After(now))
}

func boundOwnerUnavailable() error {
	return &proxymodel.Error{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "conversation owner is temporarily unavailable; continuity was preserved",
	}
}

func (router *Router) legacyBoundResponse(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	response *proxymodel.Response,
	failed bool,
) proxymodel.Forwarded {
	router.recordRouteOutcome(ctx, account.ID, request.QuotaSelection, response, responseOutcome{kind: responsePass, failed: failed})
	if response.StatusCode == http.StatusUnauthorized {
		router.quarantineAuthFailure(account.ID, time.Minute)
	}
	return proxymodel.Forwarded{Response: response, AccountID: account.ID, Failed: failed}
}

func (router *Router) handleBoundResponse(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
	response *proxymodel.Response,
) (proxymodel.Forwarded, error) {
	keys := requestRoutingAffinity(request)
	return router.handleBoundResponseWithFailure(ctx, request, accounts, account, response, false, &keys)
}

func (router *Router) handleBoundResponseWithFailure(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
	response *proxymodel.Response,
	failed bool,
	keys *affinityKeys,
) (proxymodel.Forwarded, error) {
	outcome, pending, err := router.classify(response, account.Provider.Kind)
	if err != nil {
		if pending != nil && pending.transient {
			if isTransportFailure(err) {
				router.recordTransportExecutionFailure(ctx, account.ID, request.QuotaSelection, err)
			} else {
				router.recordRouteFailure(ctx, account.ID, request.QuotaSelection)
			}
		}
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "conversation owner response failed before commitment",
		}
	}
	outcome.failed = outcome.failed || failed
	router.recordRouteOutcome(ctx, account.ID, request.QuotaSelection, response, outcome)
	if outcome.kind == responsePass {
		router.clearQuotaBlocked(account.ID)
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	router.applyRetryOutcome(ctx, account.ID, request.QuotaSelection, outcome)
	if outcome.quota && router.autoRedeem {
		redeemed, ok, redeemErr := router.tryAutoRedeem(ctx, accounts, account.ID, request)
		if redeemErr != nil {
			closePendingResponse(pending)
			return proxymodel.Forwarded{}, redeemErr
		}
		if ok {
			closePendingResponse(pending)
			return router.redeemedAttempt(ctx, request, redeemed)
		}
	}
	if result, handled, recoveryErr := router.recoverBoundRetryableFailure(
		ctx, request, accounts, account, keys, outcome, pending,
	); recoveryErr != nil || handled {
		return result, recoveryErr
	}
	return pendingForwarded(account.ID, outcome, pending), nil
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
