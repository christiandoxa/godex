package quota

import (
	"context"
	"errors"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (status *Status) runProfiles(ctx context.Context, options Options) ([]quotamodel.Report, error) {
	targets, err := status.selectedProfiles(ctx, options)
	if err != nil {
		return nil, err
	}
	reports := make([]quotamodel.Report, 0, len(targets))
	for _, target := range targets {
		report := quotamodel.Report{
			ProfileName: target.Name, Provider: target.Provider, Auth: target.Auth,
			Email: target.Email, Active: target.Active, Enabled: target.Enabled,
		}
		status.populateProfileQuota(ctx, &report, target, options.BaseURL)
		reports = append(reports, report)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if status.virtual != nil {
		reports = append(reports, virtualQuotaReports(status.virtual.Collect(ctx, options.ProviderFilter, options.BaseURL))...)
	}
	return reports, nil
}

func virtualQuotaReports(results []quotamodel.VirtualResult) []quotamodel.Report {
	reports := make([]quotamodel.Report, 0, len(results))
	for _, result := range results {
		report := quotamodel.Report{
			ProfileName: result.Name,
			Provider:    result.Provider,
			Auth:        result.Auth,
			Enabled:     true,
			External:    result.External,
			Err:         result.Err,
		}
		switch {
		case result.Err != nil:
			report.State = "error"
		case result.External != nil:
			report.Email = result.External.Account
			report.State = strings.ToLower(strings.TrimSpace(result.External.Status))
		default:
			report.State = "unsupported"
		}
		reports = append(reports, report)
	}
	return reports
}

func (status *Status) populateProfileQuota(ctx context.Context, report *quotamodel.Report, target profilemodel.QuotaTarget, baseURL string) {
	switch {
	case !target.Enabled:
		report.State = "disabled"
	case target.Provider == "openai":
		status.populateOpenAIProfileQuota(ctx, report, target, baseURL)
	case status.externalProvider(target.Provider) != nil:
		info, err := status.externalProvider(target.Provider).FetchQuota(ctx, target)
		report.Err = err
		if err != nil {
			report.State = "error"
			return
		}
		report.External = &info
		report.State = strings.ToLower(strings.TrimSpace(info.Status))
	default:
		report.State = "unsupported"
	}
}

func (status *Status) populateOpenAIProfileQuota(ctx context.Context, report *quotamodel.Report, target profilemodel.QuotaTarget, baseURL string) {
	if !target.Compatible {
		report.State = target.Auth
		return
	}
	report.Usage, report.Err = status.fetchHomeUsage(ctx, target.CodexHome, baseURL)
	report.State = quotaState(*report, status.now())
}

func (status *Status) externalProvider(provider string) externalProfileGateway {
	if status == nil || status.external == nil {
		return nil
	}
	return status.external[strings.ToLower(strings.TrimSpace(provider))]
}

func (status *Status) selectedProfiles(ctx context.Context, options Options) ([]profilemodel.QuotaTarget, error) {
	targets, err := status.profiles.QuotaTargets(ctx)
	if err != nil {
		return nil, err
	}
	if options.All {
		return filterQuotaTargets(targets, options), nil
	}
	if options.Selector != "" {
		for _, target := range targets {
			if target.Name == options.Selector {
				return []profilemodel.QuotaTarget{target}, nil
			}
		}
		return nil, errors.New("quota profile does not exist")
	}
	for _, target := range targets {
		if target.Active {
			return []profilemodel.QuotaTarget{target}, nil
		}
	}
	return nil, errors.New("no active profile")
}

func filterQuotaTargets(targets []profilemodel.QuotaTarget, options Options) []profilemodel.QuotaTarget {
	filtered := make([]profilemodel.QuotaTarget, 0, len(targets))
	for _, target := range targets {
		if providerFilterMatches(options.ProviderFilter, target.Provider) && authFilterMatches(options.AuthFilter, target) {
			filtered = append(filtered, target)
		}
	}
	return filtered
}

func providerFilterMatches(filter, provider string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	provider = strings.ToLower(strings.TrimSpace(provider))
	if filter == "" || filter == "all" {
		return true
	}
	if filter == "claude" {
		filter = "anthropic"
	}
	return filter == provider
}

func authFilterMatches(filter string, target profilemodel.QuotaTarget) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	switch filter {
	case "", "all":
		return true
	case "quota-compatible":
		return target.Compatible
	case "non-quota-compatible":
		return !target.Compatible
	default:
		return strings.EqualFold(filter, target.Auth)
	}
}

func (status *Status) selectedProfile(ctx context.Context, selector string) (profilemodel.QuotaTarget, error) {
	targets, err := status.profiles.QuotaTargets(ctx)
	if err != nil {
		return profilemodel.QuotaTarget{}, err
	}
	for _, target := range targets {
		if selector != "" && target.Name == selector {
			return target, nil
		}
		if selector == "" && target.Active {
			return target, nil
		}
	}
	return profilemodel.QuotaTarget{}, errors.New("quota profile does not exist or no profile is active")
}
