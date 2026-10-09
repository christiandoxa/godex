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
	for _, inspected := range targets {
		target := inspected.target
		if options.Selector != "" && target.Provider == "openai" &&
			strings.EqualFold(strings.TrimSpace(target.Auth), "no-auth") &&
			inspected.modelProvider == nil {
			return nil, noAuthQuotaError(target)
		}
		report := quotamodel.Report{
			ProfileName: target.Name, Provider: target.Provider, Auth: target.Auth,
			Email: target.Email, Active: target.Active, Enabled: target.Enabled,
		}
		status.populateProfileQuota(ctx, &report, inspected, options.BaseURL)
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

func (status *Status) populateProfileQuota(ctx context.Context, report *quotamodel.Report, inspected inspectedProfileQuotaTarget, baseURL string) {
	target := inspected.target
	switch {
	case !target.Enabled:
		report.State = "disabled"
	case target.Provider == "openai":
		status.populateOpenAIProfileQuota(ctx, report, inspected, baseURL)
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

func (status *Status) populateOpenAIProfileQuota(ctx context.Context, report *quotamodel.Report, inspected inspectedProfileQuotaTarget, baseURL string) {
	target := inspected.target
	if inspected.modelProviderErr != nil {
		report.Err = inspected.modelProviderErr
		report.State = "error"
		return
	}
	if inspected.modelProvider != nil {
		report.External = codexModelProviderQuota(*inspected.modelProvider)
		report.State = "configured"
		return
	}
	if !target.Compatible {
		report.State = target.Auth
		return
	}
	report.Usage, report.Err = status.fetchHomeUsage(ctx, target.CodexHome, baseURL)
	if report.Err == nil && strings.TrimSpace(baseURL) == "" {
		now := status.now()
		status.storeUsage(target.CodexHome, report.Usage, now)
		for _, key := range statusSnapshotKeys(target.Name, target.AccountID) {
			status.storeUsageSnapshot(ctx, key, report.Usage, now)
		}
	}
	report.State = quotaState(*report, status.now())
}

func (status *Status) externalProvider(provider string) externalProfileGateway {
	if status == nil || status.external == nil {
		return nil
	}
	return status.external[strings.ToLower(strings.TrimSpace(provider))]
}

func (status *Status) selectedProfiles(ctx context.Context, options Options) ([]inspectedProfileQuotaTarget, error) {
	targets, err := status.profiles.QuotaTargets(ctx)
	if err != nil {
		return nil, err
	}
	if !options.All {
		for _, target := range targets {
			if (options.Selector != "" && target.Name == options.Selector) || (options.Selector == "" && target.Active) {
				return []inspectedProfileQuotaTarget{status.inspectProfileQuotaTarget(ctx, target)}, nil
			}
		}
		if options.Selector != "" {
			return nil, errors.New("quota profile does not exist")
		}
		return nil, errors.New("no active profile")
	}

	inspected := make([]inspectedProfileQuotaTarget, 0, len(targets))
	for _, target := range targets {
		inspected = append(inspected, status.inspectProfileQuotaTarget(ctx, target))
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return filterQuotaTargets(inspected, options), nil
}

func filterQuotaTargets(targets []inspectedProfileQuotaTarget, options Options) []inspectedProfileQuotaTarget {
	filtered := make([]inspectedProfileQuotaTarget, 0, len(targets))
	for _, inspected := range targets {
		if profileProviderFilterMatches(options.ProviderFilter, inspected) && authFilterMatches(options.AuthFilter, inspected.target) {
			filtered = append(filtered, inspected)
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
