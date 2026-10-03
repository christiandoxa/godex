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
			return launchTarget(report), nil
		}
	}
	return profilemodel.LaunchTarget{}, fmt.Errorf(profileNotFoundFormat, name)
}

func launchTarget(report Report) profilemodel.LaunchTarget {
	return profilemodel.LaunchTarget{
		Name: report.Profile.Name, CodexHome: report.Profile.CodexHome,
		AccountID: report.AccountID, Provider: string(report.Profile.Provider.Kind),
		ProviderConfig: providerSnapshotFromEntity(report.Profile.Provider),
	}
}

func (catalog *Catalog) AcquireLaunch(ctx context.Context, name string) (func() error, error) {
	return catalog.profiles.Acquire(ctx, name)
}

func (catalog *Catalog) ActiveLaunch(ctx context.Context) (profilemodel.LaunchTarget, bool, error) {
	profile, active, err := catalog.ActiveStandalone(ctx)
	if err != nil || !active {
		return profilemodel.LaunchTarget{}, active, err
	}
	return profilemodel.LaunchTarget{
		Name: profile.Name, CodexHome: profile.CodexHome, Provider: string(profile.Provider.Kind),
		ProviderConfig: providerSnapshotFromEntity(profile.Provider),
	}, true, nil
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
	profile, err := catalog.profiles.Resolve(ctx, profileName)
	if err != nil {
		return "", false, err
	}
	if profile.Provider.Kind != profileentity.ProviderOpenAI {
		return "", false, nil
	}
	return catalog.profiles.ReadOpenAICompatibleBaseURL(profile.CodexHome)
}
