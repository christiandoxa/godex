package runtime

import (
	"context"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

func (launcher runSessionLauncher) recoveryQuotaReady04380(ctx context.Context, id, model string) bool {
	return launcher.options.SkipQuotaPreflight ||
		launcher.runner != nil && launcher.runner.RecoveryQuotaReadyForModel(ctx, id, model)
}

// A failed-session fallback is last-resort only, after a completed bounded
// wait. Its identity must still be an enabled, authenticated OpenAI profile
// in the trusted profile catalog; a missing quota source, an identity that
// differs from the request owner, or a pre-existing attempt fails closed.
func (launcher runSessionLauncher) recoveryFailedProfileAfterRound04380(
	ctx context.Context, report sessionmodel.Report, attempted map[string]bool,
) (profilemodel.LaunchTarget, bool) {
	source, ok := launcher.profiles.(recoveryProfileSource04360)
	if !ok || ctx == nil || ctx.Err() != nil {
		return profilemodel.LaunchTarget{}, false
	}
	original, ok := canonicalRecoveryAccountID04360(ctx, source, report.AccountID)
	if !ok || original == "" || attempted[original] {
		return profilemodel.LaunchTarget{}, false
	}
	if report.UpstreamAccountID != "" {
		upstream, valid := canonicalRecoveryAccountID04360(ctx, source, report.UpstreamAccountID)
		if !valid || upstream != original {
			return profilemodel.LaunchTarget{}, false
		}
	}
	homes, err := source.SessionProfiles(ctx)
	if err != nil || len(homes) < 2 || ctx.Err() != nil {
		return profilemodel.LaunchTarget{}, false
	}
	for _, home := range homes {
		if !home.Enabled || home.Provider != "openai" || home.AccountID != original {
			continue
		}
		target, err := launcher.profiles.ResolveLaunch(ctx, home.Name)
		if err != nil || ctx.Err() != nil || target.AccountID != original || target.Provider != "openai" ||
			!strings.EqualFold(strings.TrimSpace(target.Auth), "chatgpt") {
			return profilemodel.LaunchTarget{}, false
		}
		if !launcher.recoveryQuotaReady04380(ctx, original, report.LastModel) {
			return profilemodel.LaunchTarget{}, false
		}
		return target, true
	}
	return profilemodel.LaunchTarget{}, false
}
