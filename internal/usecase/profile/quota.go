package profile

import (
	"context"
	"errors"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func (catalog *Catalog) QuotaTargets(ctx context.Context) ([]profilemodel.QuotaTarget, error) {
	reports, err := catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]profilemodel.QuotaTarget, 0, len(reports))
	for _, report := range reports {
		auth, err := catalog.quotaAuthSummary(ctx, report.Profile)
		if err != nil {
			return nil, err
		}
		targets = append(targets, profilemodel.QuotaTarget{
			Name: report.Profile.Name, CodexHome: report.Profile.CodexHome,
			Email: report.Profile.Email, Provider: string(report.Profile.Provider.Kind),
			ProviderConfig: providerSnapshotFromEntity(report.Profile.Provider),
			Auth:           auth.Label, AccountID: report.AccountID,
			Active: report.Active, Enabled: report.Enabled, Compatible: auth.Compatible,
		})
	}
	return targets, nil
}

func (catalog *Catalog) quotaAuthSummary(ctx context.Context, profile profileentity.Profile) (profilemodel.QuotaAuthSummary, error) {
	switch profile.Provider.Kind {
	case profileentity.ProviderOpenAI:
		if catalog.quotaAuth == nil {
			return profilemodel.QuotaAuthSummary{}, errors.New("profile quota auth inspection is not configured")
		}
		return catalog.quotaAuth.InspectQuotaAuth(ctx, profile.CodexHome)
	case profileentity.ProviderGemini:
		return profilemodel.QuotaAuthSummary{Label: "gemini-oauth-disabled"}, nil
	case profileentity.ProviderAnthropic:
		return profilemodel.QuotaAuthSummary{Label: "claude-oauth"}, nil
	case profileentity.ProviderCopilot:
		return profilemodel.QuotaAuthSummary{Label: "copilot"}, nil
	case profileentity.ProviderKiro:
		return profilemodel.QuotaAuthSummary{Label: "kiro"}, nil
	case profileentity.ProviderDeepSeek, profileentity.ProviderLocal:
		return profilemodel.QuotaAuthSummary{Label: "api-key"}, nil
	case profileentity.ProviderAgy:
		return profilemodel.QuotaAuthSummary{Label: "agy"}, nil
	default:
		return profilemodel.QuotaAuthSummary{Label: "unknown"}, nil
	}
}
