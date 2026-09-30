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
		return proxymodel.Forwarded{Response: response, AccountID: owner}, nil
	}
	return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 409, Message: "conversation owner is no longer registered; continuity was preserved"}
}

func (router *Router) forwardFresh(ctx context.Context, request proxymodel.Request, accounts []proxymodel.Account) (proxymodel.Forwarded, error) {
	candidates := router.candidates(accounts, router.now())
	if len(candidates) == 0 {
		return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 503, Message: "no enabled account is available"}
	}
	var last *pendingResponse
	defer func() {
		if last != nil {
			last.close()
		}
	}()
	for _, account := range candidates {
		response, err := router.execute(ctx, request, account)
		if err != nil {
			if ctx.Err() != nil {
				return proxymodel.Forwarded{}, ctx.Err()
			}
			continue
		}
		outcome, pending, err := router.classify(response)
		if err != nil {
			pending.close()
			return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 502, Message: "upstream response failed before commitment"}
		}
		if last != nil {
			last.close()
			last = nil
		}
		if outcome.kind == responsePass {
			return proxymodel.Forwarded{Response: response, Prefix: pending.prefix, AccountID: account.ID, Failed: outcome.failed}, nil
		}
		if outcome.kind == responseAuthFailure {
			router.quarantineAccount(account.ID, 60e9)
		} else if outcome.quarantine > 0 {
			router.quarantineAccount(account.ID, outcome.quarantine)
		}
		pending.accountID = account.ID
		last = pending
	}
	if last != nil {
		result := proxymodel.Forwarded{Response: last.response, Prefix: last.prefix, AccountID: last.accountID, Failed: true}
		last = nil
		return result, nil
	}
	return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 502, Message: "all eligible accounts failed before upstream response commitment"}
}
