package profile

import (
	"context"
	"errors"
	"fmt"
	"sort"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func (catalog *Catalog) ActiveStandalone(ctx context.Context) (profileentity.Profile, bool, error) {
	var current profileentity.Profile
	active := false
	err := catalog.withBundleImportLock(ctx, func() error {
		hasActive, err := catalog.profiles.HasActive(ctx)
		if err != nil || !hasActive {
			return err
		}
		current, err = catalog.profiles.Current(ctx)
		active = err == nil
		return err
	})
	return current, active, err
}

func (catalog *Catalog) ResolveLaunch(ctx context.Context, name string) (profilemodel.LaunchTarget, error) {
	listed, err := catalog.List(ctx)
	if err != nil {
		return profilemodel.LaunchTarget{}, err
	}
	for _, report := range listed {
		if report.Profile.Name == name {
			return catalog.launchTargetWithAuth(ctx, report)
		}
	}
	return profilemodel.LaunchTarget{}, fmt.Errorf(profileNotFoundFormat, name)
}

func (catalog *Catalog) launchTargetWithAuth(ctx context.Context, report Report) (profilemodel.LaunchTarget, error) {
	target := launchTarget(report)
	if report.Profile.Provider.Kind == profileentity.ProviderOpenAI && catalog.quotaAuth != nil {
		auth, err := catalog.quotaAuthSummary(ctx, report.Profile)
		if err != nil {
			return profilemodel.LaunchTarget{}, err
		}
		target.Auth = auth.Label
	}
	return target, nil
}

func launchTarget(report Report) profilemodel.LaunchTarget {
	return profilemodel.LaunchTarget{
		Name: report.Profile.Name, CodexHome: report.Profile.CodexHome,
		AccountID: report.AccountID, Provider: string(report.Profile.Provider.Kind),
		ProviderConfig: providerSnapshotFromEntity(report.Profile.Provider),
	}
}

func (catalog *Catalog) AcquireLaunch(ctx context.Context, name string) (func() error, error) {
	listed, err := catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, report := range listed {
		if report.Profile.Name != name {
			continue
		}
		if report.AccountID != "" {
			leases, ok := catalog.accounts.(interface {
				AcquireProfiles(context.Context, []string) (func() error, error)
			})
			if !ok {
				return nil, errors.New("account profile lease support is not configured")
			}
			return leases.AcquireProfiles(ctx, []string{report.AccountID})
		}
		return catalog.profiles.Acquire(ctx, name)
	}
	return nil, fmt.Errorf(profileNotFoundFormat, name)
}

func (catalog *Catalog) AcquireOpenAILaunch(ctx context.Context, name string) (func() error, string, bool, string, error) {
	var release func() error
	var baseURL, authLabel string
	var compatible bool
	err := catalog.withBundleImportLock(ctx, func() error {
		listed, err := catalog.list(ctx)
		if err != nil {
			return err
		}
		var target Report
		found := false
		for _, report := range listed {
			if report.Profile.Name == name {
				target, found = report, true
				break
			}
		}
		if !found {
			return fmt.Errorf(profileNotFoundFormat, name)
		}
		if target.Profile.Provider.Kind != profileentity.ProviderOpenAI {
			return fmt.Errorf("profile %q does not use provider %q", name, profileentity.ProviderOpenAI)
		}
		if target.AccountID != "" {
			leases, ok := catalog.accounts.(interface {
				AcquireProfiles(context.Context, []string) (func() error, error)
			})
			if !ok {
				return errors.New("account profile lease support is not configured")
			}
			release, err = leases.AcquireProfiles(ctx, []string{target.AccountID})
		} else {
			release, err = catalog.profiles.Acquire(ctx, target.Profile.Name)
		}
		if err != nil {
			return err
		}
		baseURL, compatible, err = catalog.profiles.ReadOpenAICompatibleBaseURL(target.Profile.CodexHome)
		if err != nil {
			_ = release()
			release = nil
			return err
		}
		if catalog.quotaAuth != nil {
			auth, authErr := catalog.quotaAuthSummary(ctx, target.Profile)
			if authErr != nil {
				_ = release()
				release = nil
				return authErr
			}
			authLabel = auth.Label
		}
		return nil
	})
	if err != nil {
		return nil, "", false, "", err
	}
	return release, baseURL, compatible, authLabel, nil
}

func (catalog *Catalog) CurrentLaunch(ctx context.Context) (profilemodel.LaunchTarget, error) {
	report, err := catalog.Current(ctx)
	if err != nil {
		return profilemodel.LaunchTarget{}, err
	}
	return catalog.launchTargetWithAuth(ctx, report)
}

func (catalog *Catalog) ActiveLaunch(ctx context.Context) (profilemodel.LaunchTarget, bool, error) {
	var report Report
	var target profilemodel.LaunchTarget
	found := false
	err := catalog.withBundleImportLock(ctx, func() error {
		hasProfile, err := catalog.profiles.HasActive(ctx)
		if err != nil {
			return err
		}
		if hasProfile {
			profile, err := catalog.profiles.Current(ctx)
			if err != nil {
				return err
			}
			report = Report{Profile: profile, Active: true, Enabled: true}
			found = true
		} else {
			activeID, err := catalog.accounts.ActiveID(ctx)
			if err != nil {
				return err
			}
			if activeID == "" {
				return nil
			}
			accounts, err := catalog.accounts.List(ctx)
			if err != nil {
				return err
			}
			for _, account := range accounts {
				if account.ID == activeID {
					report = Report{
						Profile: accountProfile(account, catalog.accounts.CodexHome(account.ID)),
						Active:  true, Enabled: account.Enabled, AccountID: account.ID,
					}
					found = true
					break
				}
			}
			if !found {
				return errors.New("active account metadata is inconsistent")
			}
		}
		target, err = catalog.launchTargetWithAuth(ctx, report)
		return err
	})
	if err != nil {
		return profilemodel.LaunchTarget{}, false, err
	}
	if !found || report.AccountID != "" && target.Auth != "api-key" {
		return profilemodel.LaunchTarget{}, false, nil
	}
	return target, true, nil
}

func (catalog *Catalog) ResolveProviderLaunch(
	ctx context.Context,
	provider, requested string,
) (profilemodel.LaunchTarget, bool, error) {
	if requested != "" {
		target, err := catalog.ResolveLaunch(ctx, requested)
		return target, err == nil, err
	}
	active, activeFound, err := catalog.ActiveLaunch(ctx)
	if err != nil {
		return profilemodel.LaunchTarget{}, false, err
	}
	if activeFound && active.Provider == provider {
		return active, true, nil
	}
	listed, err := catalog.List(ctx)
	if err != nil {
		return profilemodel.LaunchTarget{}, false, err
	}
	for _, report := range listed {
		if report.AccountID == "" && report.Enabled && string(report.Profile.Provider.Kind) == provider {
			return launchTarget(report), true, nil
		}
	}
	if activeFound {
		return active, true, nil
	}
	for _, report := range listed {
		if report.AccountID == "" && report.Enabled && report.Profile.CodexHome != "" {
			return launchTarget(report), true, nil
		}
	}
	return profilemodel.LaunchTarget{}, false, nil
}

func (catalog *Catalog) ProviderLaunchPool(
	ctx context.Context,
	selectedName, provider string,
	allowRotate bool,
) ([]profilemodel.LaunchTarget, error) {
	selected, err := catalog.ResolveLaunch(ctx, selectedName)
	if err != nil {
		return nil, err
	}
	if selected.Provider != provider {
		return nil, fmt.Errorf("profile %q does not use provider %q", selectedName, provider)
	}
	if !allowRotate {
		return []profilemodel.LaunchTarget{selected}, nil
	}
	listed, err := catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	result := []profilemodel.LaunchTarget{selected}
	for _, report := range listed {
		if report.Profile.Name == selectedName || report.AccountID != "" ||
			string(report.Profile.Provider.Kind) != provider {
			continue
		}
		result = append(result, launchTarget(report))
	}
	sort.Slice(result[1:], func(i, j int) bool {
		return result[i+1].Name < result[j+1].Name
	})
	return result, nil
}

func (catalog *Catalog) AcquireLaunchPool(
	ctx context.Context,
	names []string,
) (func() error, error) {
	names = append([]string(nil), names...)
	sort.Strings(names)
	releases := make([]func() error, 0, len(names))
	for _, name := range names {
		release, err := catalog.profiles.Acquire(ctx, name)
		if err != nil {
			for index := len(releases) - 1; index >= 0; index-- {
				_ = releases[index]()
			}
			return nil, err
		}
		releases = append(releases, release)
	}
	return func() error {
		var releaseErr error
		for index := len(releases) - 1; index >= 0; index-- {
			releaseErr = errors.Join(releaseErr, releases[index]())
		}
		return releaseErr
	}, nil
}

func (catalog *Catalog) OpenAICompatibleBaseURL(ctx context.Context, profileName string) (string, bool, error) {
	target, err := catalog.ResolveLaunch(ctx, profileName)
	if err != nil {
		return "", false, err
	}
	if target.Provider != string(profileentity.ProviderOpenAI) {
		return "", false, nil
	}
	return catalog.profiles.ReadOpenAICompatibleBaseURL(target.CodexHome)
}
