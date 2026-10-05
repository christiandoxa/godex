package routing

import (
	"context"
	"errors"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (router *Router) tryAutoRedeem(
	ctx context.Context,
	accounts []proxymodel.Account,
	preferredID string,
	request proxymodel.Request,
) (proxymodel.Account, bool, error) {
	if !router.autoRedeem || router.redeemer == nil {
		return proxymodel.Account{}, false, nil
	}
	pool := router.autoRedeemAccounts(accounts, preferredID)
	if len(pool) == 0 {
		return proxymodel.Account{}, false, nil
	}
	accountID, redeemed, err := router.redeemer.Try(ctx, pool, preferredID, request)
	if err != nil {
		return proxymodel.Account{}, false, err
	}
	if !redeemed || accountID == "" {
		return proxymodel.Account{}, false, nil
	}
	for _, account := range pool {
		if account.ID != accountID {
			continue
		}
		account.EligibleAfter = time.Time{}
		router.clearQuarantine(account.ID)
		return account, true, nil
	}
	return proxymodel.Account{}, false, errors.New("auto-redeem returned an unknown runtime account")
}

func (router *Router) autoRedeemAccounts(accounts []proxymodel.Account, preferredID string) []proxymodel.Account {
	now := router.now()
	result := make([]proxymodel.Account, 0, len(accounts))
	for _, account := range accounts {
		if !account.Enabled || account.ID == "" || account.Home == "" || externalProviderKind(account.Provider.Kind) {
			continue
		}
		if preferredID != "" && account.ID != preferredID {
			continue
		}
		if preferredID == "" && router.isQuarantined(account.ID, now) && !router.quotaBlockedAccount(account.ID) {
			continue
		}
		result = append(result, account)
	}
	return result
}

func (router *Router) redeemedAttempt(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (proxymodel.Forwarded, error) {
	response, err := router.executeWithProfileInflightWait(ctx, request, account, false)
	if err != nil {
		if ctx.Err() != nil {
			return proxymodel.Forwarded{}, ctx.Err()
		}
		return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 502, Message: "auto-redeemed account could not be reached"}
	}
	outcome, pending, err := router.classify(response, account.Provider.Kind)
	if err != nil {
		if pending != nil {
			pending.close()
		}
		return proxymodel.Forwarded{}, &proxymodel.Error{StatusCode: 502, Message: "auto-redeem retry failed before response commitment"}
	}
	if outcome.kind == responsePass {
		router.clearQuotaBlocked(account.ID)
		if retryBackoffCommitSuccess(response, outcome) {
			router.clearRetryBackoff(ctx, account.ID)
		}
	} else {
		router.applyRetryOutcome(ctx, account.ID, outcome)
	}
	return proxymodel.Forwarded{
		Response: pending.response, Prefix: pending.prefix, AccountID: account.ID,
		Failed: outcome.failed || outcome.kind != responsePass,
	}, nil
}
