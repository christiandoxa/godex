package routing

import (
	"context"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (router *Router) forwardBound(ctx context.Context, request proxymodel.Request, accounts []proxymodel.Account, owner string) (proxymodel.Forwarded, error) {
	for _, account := range accounts {
		if account.ID != owner {
			continue
		}
		if !account.Enabled || router.isQuarantined(owner, router.now()) {
			return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 503, Message: "conversation owner is temporarily unavailable; continuity was preserved"}
		}
		response, err := router.execute(ctx, request, account)
		if err != nil {
			return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 502, Message: "conversation owner could not be reached; continuity was preserved"}
		}
		if response.StatusCode == http.StatusUnauthorized {
			router.quarantineAccount(owner, 60e9)
		}
		return proxymodel.Forwarded{Response: response, AccountID: account.ID}, nil
	}
	return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 409, Message: "conversation owner is no longer registered; continuity was preserved"}
}

func (router *Router) forwardFresh(ctx context.Context, request proxymodel.Request, accounts []proxymodel.Account) (proxymodel.Forwarded, error) {
	candidates := router.candidates(accounts, router.now())
	if len(candidates) == 0 {
		return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 503, Message: "no enabled account is available"}
	}
	var last *pendingResponse
	defer closePending(&last)
	for _, account := range candidates {
		result, pending, err := router.freshAttempt(ctx, request, account)
		if err != nil {
			return proxymodel.Forwarded{}, err
		}
		if result != nil {
			return *result, nil
		}
		if pending != nil {
			replacePending(&last, pending)
		}
	}
	return finishFresh(&last)
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
		result := &proxymodel.Forwarded{Response: response, Prefix: pending.prefix, AccountID: account.ID, Failed: outcome.failed}
		return result, nil, nil
	}
	router.applyRetryOutcome(account.ID, outcome)
	pending.accountID = account.ID
	return nil, pending, nil
}

func (router *Router) applyRetryOutcome(accountID string, outcome responseOutcome) {
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
