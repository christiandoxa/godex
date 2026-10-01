package profile

import (
	"context"
	"fmt"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func (catalog *Catalog) ActiveStandalone(ctx context.Context) (profileentity.Profile, bool, error) {
	hasActive, err := catalog.profiles.HasActive(ctx)
	if err != nil {
		return profileentity.Profile{}, false, err
	}
	if !hasActive {
		return profileentity.Profile{}, false, nil
	}
	current, err := catalog.profiles.Current(ctx)
	if err != nil {
		return profileentity.Profile{}, false, err
	}
	return current, true, nil
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
