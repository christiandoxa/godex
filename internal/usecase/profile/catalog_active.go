package profile

import (
	"context"
	"errors"
	"fmt"
	"sort"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
)

func (catalog *Catalog) List(ctx context.Context) ([]Report, error) {
	var reports []Report
	err := catalog.withBundleImportLock(ctx, func() error {
		var err error
		reports, err = catalog.list(ctx)
		return err
	})
	return reports, err
}

func (catalog *Catalog) list(ctx context.Context) ([]Report, error) {
	stored, err := catalog.profiles.List(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := catalog.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	activeName := catalog.activeName(ctx)
	byName := make(map[string]Report, len(stored)+len(accounts))
	for _, current := range accounts {
		profile := accountProfile(current, catalog.accounts.CodexHome(current.ID))
		byName[profile.Name] = Report{Profile: profile, Active: profile.Name == activeName, Enabled: current.Enabled, AccountID: current.ID}
	}
	for _, current := range stored {
		if _, exists := byName[current.Name]; exists {
			return nil, fmt.Errorf("profile %q conflicts with a managed account", current.Name)
		}
		byName[current.Name] = Report{Profile: current, Active: current.Name == activeName, Enabled: true}
	}
	result := make([]Report, 0, len(byName))
	for _, current := range byName {
		result = append(result, current)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Profile.Name < result[j].Profile.Name })
	return result, nil
}

func (catalog *Catalog) Current(ctx context.Context) (Report, error) {
	var report Report
	err := catalog.withBundleImportLock(ctx, func() error {
		var err error
		report, err = catalog.current(ctx)
		return err
	})
	return report, err
}

func (catalog *Catalog) current(ctx context.Context) (Report, error) {
	hasActive, err := catalog.profiles.HasActive(ctx)
	if err != nil {
		return Report{}, err
	}
	if hasActive {
		current, err := catalog.profiles.Current(ctx)
		if err != nil {
			return Report{}, err
		}
		return Report{Profile: current, Active: true, Enabled: true}, nil
	}
	account, err := catalog.accounts.Current(ctx)
	if err != nil {
		return Report{}, errors.New("no active profile")
	}
	return Report{Profile: accountProfile(account, catalog.accounts.CodexHome(account.ID)), Active: true, Enabled: account.Enabled, AccountID: account.ID}, nil
}

func (catalog *Catalog) Use(ctx context.Context, name string) (Report, error) {
	var report Report
	err := catalog.withBundleImportLock(ctx, func() error {
		var err error
		report, err = catalog.use(ctx, name)
		return err
	})
	return report, err
}

func (catalog *Catalog) use(ctx context.Context, name string) (Report, error) {
	if current, err := catalog.profiles.Resolve(ctx, name); err == nil {
		selected, setErr := catalog.profiles.SetActive(ctx, current.Name)
		return Report{Profile: selected, Active: setErr == nil, Enabled: true}, setErr
	}
	accounts, err := catalog.accounts.List(ctx)
	if err != nil {
		return Report{}, err
	}
	for _, account := range accounts {
		if account.Name != name {
			continue
		}
		previous, previousErr := catalog.profiles.Current(ctx)
		if err := catalog.profiles.ClearActive(ctx); err != nil {
			return Report{}, err
		}
		selected, err := catalog.accounts.SetActive(ctx, account.Name)
		if err != nil {
			if previousErr == nil {
				_, _ = catalog.profiles.SetActive(ctx, previous.Name)
			}
			return Report{}, err
		}
		profile := accountProfile(selected, catalog.accounts.CodexHome(selected.ID))
		return Report{Profile: profile, Active: true, Enabled: selected.Enabled, AccountID: selected.ID}, nil
	}
	return Report{}, fmt.Errorf(profileNotFoundFormat, name)
}

func (catalog *Catalog) activeName(ctx context.Context) string {
	if hasActive, err := catalog.profiles.HasActive(ctx); err == nil && hasActive {
		if current, currentErr := catalog.profiles.Current(ctx); currentErr == nil {
			return current.Name
		}
	}
	if account, err := catalog.accounts.Current(ctx); err == nil {
		return account.Name
	}
	return ""
}

func accountProfile(account accountentity.Account, home string) profileentity.Profile {
	return profileentity.Profile{
		Name: account.Name, CodexHome: home, Managed: true, Email: account.Email,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
}
