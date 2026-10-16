package runtime

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// RecoveryQuotaReady is a read-only, availability-checked claim for native
// session-recovery planning. The 0.438.0 tagged planner may reselect a profile
// after an exhausted transient retry round only when it is actually ready;
// absence of a quota source, cancelled reads and missing/disabled accounts
// never create synthetic readiness or release an old session binding.
// RecoveryQuotaReady retains the simple account-level probe for callers that
// have not yet supplied a trustworthy session model selection.
func (runner *Runner) RecoveryQuotaReady(ctx context.Context, accountID string) bool {
	return runner.RecoveryQuotaReadyForModel(ctx, accountID, "")
}

func (runner *Runner) RecoveryQuotaReadyForModel(ctx context.Context, accountID, model string) bool {
	if runner == nil || runner.quota == nil || runner.accounts == nil ||
		accountID == "" || ctx == nil || ctx.Err() != nil {
		return false
	}
	accounts, err := runner.accounts.List(ctx)
	if err != nil || ctx.Err() != nil {
		return false
	}
	for _, account := range accounts {
		if account.ID != accountID || !account.Enabled {
			continue
		}
		if route, ok := runner.quota.(interface {
			AvailabilityForRoute(context.Context, accountentity.Account, quotamodel.Selection) (quotamodel.Availability, error)
		}); ok {
			ready, readErr := route.AvailabilityForRoute(ctx, account, quotamodel.Selection{
				RouteKind: quotamodel.RouteKindResponses, RequestedModel: model,
			})
			return readErr == nil && ready.Ready && ready.Source != quotamodel.SourceUnknown && ctx.Err() == nil
		}
		ready, readErr := runner.quota.Ready(ctx, account)
		return readErr == nil && ready && ctx.Err() == nil
	}
	return false
}

// Legacy transient backup rounds without a quota adapter keep their previous
// bounded retry semantics. They must not, however, qualify the failed
// original owner for recovery without a ready receipt.
func (runner *Runner) RecoveryQuotaPreflightConfigured() bool {
	return runner != nil && runner.quota != nil
}
