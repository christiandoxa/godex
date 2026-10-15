package quota

import (
	"context"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// The persistence adapter is optional. It exposes authenticated-failure
// *metadata* only; no bearer/token data reaches the quota use case.
type inheritedAuthPenaltySource043714 interface {
	LoadAuthBackoffs(context.Context, time.Time) ([]routingentity.AuthBackoff, error)
}

func (status *Status) SetInheritedAuthPenaltySource043714(source inheritedAuthPenaltySource043714) {
	if status != nil {
		status.inheritedAuthPenalty = source
	}
}

// Existing warm-up skips fresh quota DTOs; Prodex 0.437.14 must not skip a
// real quota HTTP revalidation merely because the inherited cache is fresh
// while an independent historical 401/403 penalty is still active.
func (status *Status) startupInheritedAuthProbes043714(
	ctx context.Context, targets []startupProbeTarget, baseURL string, noProxy bool,
) (map[string]bool, int) {
	done := make(map[string]bool)
	if status == nil || status.inheritedAuthPenalty == nil || ctx == nil || ctx.Err() != nil {
		return done, 0
	}
	backoffs, err := status.inheritedAuthPenalty.LoadAuthBackoffs(ctx, status.now())
	if err != nil {
		return done, 0
	}
	byID := make(map[string]struct{}, len(backoffs))
	for _, backoff := range backoffs {
		byID[backoff.AccountID] = struct{}{}
	}
	used := 0
	for _, target := range targets {
		if used >= startupProbeWarmLimit || ctx.Err() != nil {
			break
		}
		if !target.enabled || target.id == "" || target.home == "" {
			continue
		}
		if _, matched := byID[target.id]; !matched {
			continue
		}
		if done[target.id] {
			continue
		}
		// Schedule a credential-bound receipt, not a plain quota DTO read.
		// The empty selection marks this startup proof as route-agnostic;
		// admission derives the requested route and model after the probe.
		if err := status.scheduleAuthenticatedRecoveryProbeAtHome043714(
			ctx, target.id, target.home, quotamodel.Selection{}, baseURL, noProxy, true,
		); err == nil {
			done[target.id] = true
			used++
		}
	}
	return done, used
}
