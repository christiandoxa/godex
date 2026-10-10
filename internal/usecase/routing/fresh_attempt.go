package routing

import (
	"context"
	"errors"
	"net/http"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const upstreamTransportFailureMessage = "Runtime proxy could not secure a healthy upstream profile before the pre-commit retry budget was exhausted. Retry the request."

func (router *Router) tryFreshCandidates(
	ctx context.Context,
	request proxymodel.Request,
	candidates []proxymodel.Account,
	last **pendingResponse,
	excluded, retryable map[string]bool,
	firstEventRetryUsed *bool,
	saturatedAccounts *[]proxymodel.Account,
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
		router.recordSelectionMarker(ctx, "selection_pick", account, request.QuotaSelection)
		router.recordRouteDecisionSelected(ctx, request, account)
		result, found, pending, saturated, err := router.tryFreshCandidate(ctx, request, candidates, account)
		if err != nil || found {
			return result, found, sawTransient, sawSaturated, err
		}
		if saturated {
			sawSaturated = true
			*saturatedAccounts = append(*saturatedAccounts, account)
			continue
		}
		excluded[account.ID] = true
		if pending != nil {
			if pending.firstEventRetry {
				*firstEventRetryUsed = true
			}
			if pending.authFailure || pending.quota || pending.profileUnavailable || pending.previousResponseNotFound {
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
	result, pending, saturated, err := router.freshAttempt(
		ctx, request, account, len(accounts) == 1 && externalProviderKind(account.Provider.Kind),
	)
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
		result, retryPending, saturated, err := router.freshAttempt(
			ctx, request, redeemed, len(accounts) == 1 && externalProviderKind(redeemed.Provider.Kind),
		)
		if err != nil {
			if ctx.Err() != nil {
				return proxymodel.Forwarded{}, false, nil, ctx.Err()
			}
			return proxymodel.Forwarded{}, false, nil, nil
		}
		if saturated {
			if err := router.waitForProfileInflight(ctx, request, []proxymodel.Account{redeemed}); err != nil {
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
	terminalExternalFailure bool,
) (*proxymodel.Forwarded, *pendingResponse, bool, error) {
	response, acquired, err := router.tryExecuteWithProfileInflight(ctx, request, account, false)
	if !acquired {
		return nil, nil, true, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, false, ctx.Err()
		}
		if isProxyPreparationError(err) {
			return nil, nil, false, err
		}
		transport := isTransportFailure(err)
		if transport {
			router.recordTransportExecutionFailure(ctx, account.ID, request.QuotaSelection, err)
		} else {
			router.recordRouteFailure(ctx, account.ID, request.QuotaSelection)
		}
		return nil, &pendingResponse{accountID: account.ID, transient: true, transport: transport}, false, nil
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
			transport := isTransportFailure(err)
			if transport {
				router.recordTransportExecutionFailure(ctx, account.ID, request.QuotaSelection, err)
			} else {
				router.recordRouteFailure(ctx, account.ID, request.QuotaSelection)
			}
			pending.close()
			return nil, &pendingResponse{accountID: account.ID, transient: true, transport: transport}, false, nil
		}
		if pending != nil {
			pending.close()
		}
		return nil, nil, false, &proxymodel.Error{StatusCode: 502, Message: "upstream response failed before commitment"}
	}
	if outcome.previousResponseNotFound && request.QuotaSelection.RouteKind == quotamodel.RouteKindResponses {
		keys := requestRoutingAffinity(request)
		var rotate bool
		var recoveryErr error
		response, outcome, pending, rotate, recoveryErr = router.handleResponsesPreviousResponseNotFound(
			ctx, request, account, response, outcome, pending, &keys,
		)
		if recoveryErr != nil {
			return nil, nil, false, recoveryErr
		}
		if rotate {
			pending.accountID = account.ID
			pending.previousResponseNotFound = true
			return nil, pending, false, nil
		}
	}
	if terminalExternalFailure && outcome.kind == responseRetry &&
		(response.StatusCode == http.StatusTooManyRequests ||
			response.StatusCode == http.StatusInternalServerError ||
			response.StatusCode == http.StatusBadGateway ||
			response.StatusCode == http.StatusServiceUnavailable) {
		// Prodex returns the original upstream 429/500/503 when a lone
		// external credential has no alternate model or key to try. Waiting
		// for the same credential would cause a false Codex client timeout.
		// The next independent request remains eligible for the same key.
		router.recordRouteOutcome(ctx, account.ID, request.QuotaSelection, response,
			responseOutcome{kind: responsePass, failed: true})
		return &proxymodel.Forwarded{
			Response: response, Prefix: pending.prefix, AccountID: account.ID, Failed: true,
		}, nil, false, nil
	}
	if outcome.kind == responsePass {
		router.clearQuotaBlocked(account.ID)
		router.recordRouteOutcome(ctx, account.ID, request.QuotaSelection, response, outcome)
		result := &proxymodel.Forwarded{Response: response, Prefix: pending.prefix, AccountID: account.ID, Failed: outcome.failed}
		return result, nil, false, nil
	}
	router.recordRouteOutcome(ctx, account.ID, request.QuotaSelection, response, outcome)
	router.applyRetryOutcomeForAccount(ctx, account, request.QuotaSelection, outcome)
	pending.firstEventRetry = outcome.firstEventRetry
	pending.accountID = account.ID
	pending.authFailure = outcome.kind == responseAuthFailure
	pending.quota = outcome.quota
	pending.profileUnavailable = outcome.profileUnavailable
	pending.transient = outcome.transient
	return nil, pending, false, nil
}

func isProxyPreparationError(err error) bool {
	var presentation *proxymodel.Error
	return errors.As(err, &presentation) &&
		presentation.StatusCode == http.StatusBadGateway &&
		presentation.Message == "proxied request could not be prepared"
}

// applyRetryOutcome preserves historical durable retry backoff for managed
// profile IDs. Call applyRetryOutcomeForAccount when launch-origin metadata is
// available, so ephemeral API keys do not poison subsequent process launches.
func (router *Router) applyRetryOutcome(ctx context.Context, accountID string, selection quotamodel.Selection, outcome responseOutcome) {
	router.applyRetryOutcomeWithPersistence(ctx, accountID, selection, outcome, true)
}

func (router *Router) applyRetryOutcomeForAccount(ctx context.Context, account proxymodel.Account, selection quotamodel.Selection, outcome responseOutcome) {
	router.applyRetryOutcomeWithPersistence(ctx, account.ID, selection, outcome, !account.EphemeralAPIKey)
}

func (router *Router) applyRetryOutcomeWithPersistence(ctx context.Context, accountID string, selection quotamodel.Selection, outcome responseOutcome, persist bool) {
	duration := outcome.quarantine
	if outcome.quota {
		duration = router.quotaQuarantineDuration(accountID, selection, outcome.quotaResetAt)
		router.markQuotaBlocked(accountID)
		router.cacheQuotaFailure(accountID, selection, duration)
	} else {
		router.clearQuotaBlocked(accountID)
	}
	if outcome.kind == responseAuthFailure {
		router.quarantineAuthFailure(accountID, 60*time.Second)
		return
	}
	if outcome.kind == responseRetry && !outcome.transport {
		if duration <= 0 && !outcome.explicitRetryAdvice {
			duration = defaultProfileRetryBackoff
		}
		if outcome.explicitRetryAdvice && duration == 0 {
			// An explicit Retry-After: 0 means immediate eligibility; do
			// not accidentally restore the 20-second default quarantine.
			return
		}
		if persist {
			router.persistRetryBackoff(ctx, accountID, duration)
		} else {
			// A transient API-key pool still quarantines the failed key for
			// the current runtime, but has no durable rotation cooldown.
			router.quarantineAccount(accountID, duration)
		}
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
		if pending.transport {
			return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: http.StatusServiceUnavailable, Message: upstreamTransportFailureMessage}
		}
		return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 502, Message: "all eligible accounts failed before upstream response commitment"}
	}
	if pending.previousResponseNotFound {
		pending.close()
		return proxymodel.Forwarded{Response: staleResponsesContinuationResponse(), AccountID: pending.accountID, Failed: true}, nil
	}
	pending.commitStream()
	return proxymodel.Forwarded{Response: pending.response, Prefix: pending.prefix, AccountID: pending.accountID, Failed: true}, nil
}
