package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func (catalog *Catalog) SelectedLoginStatus(
	ctx context.Context,
	name string,
	run func(string) error,
) error {
	if run == nil {
		return errors.New("selected login status runner is not configured")
	}
	return catalog.withBundleImportLock(ctx, func() error {
		target, err := catalog.selectedOpenAITargetLocked(ctx, name)
		if err != nil {
			return err
		}
		return run(target.Profile.CodexHome)
	})
}

func (catalog *Catalog) SelectedOpenAILogin(
	ctx context.Context,
	name string,
	login func() ([]byte, error),
) (Report, error) {
	if login == nil {
		return Report{}, errors.New("selected login runner is not configured")
	}
	var result Report
	err := catalog.withBundleImportLock(ctx, func() error {
		target, err := catalog.selectedOpenAITargetLocked(ctx, name)
		if err != nil {
			return err
		}
		authJSON, err := login()
		if err != nil {
			return err
		}
		defer clearBundleBytes(authJSON)
		if catalog.auth == nil {
			return errors.New("profile auth inspection is not configured")
		}
		identity, err := catalog.auth.InspectAuthJSON(ctx, authJSON)
		if err != nil {
			return errors.New("selected login produced invalid OpenAI authentication")
		}
		current, err := catalog.selectedOpenAITargetLocked(ctx, name)
		if err != nil || current.AccountID != target.AccountID || current.Profile != target.Profile {
			return fmt.Errorf("profile %q changed while login was running", name)
		}

		desired := target.Profile
		if email := strings.TrimSpace(identity.Email); email != "" {
			desired.Email = email
		}
		source := profilemodel.ExportedProfile{
			Name: target.Profile.Name, SourceManaged: target.Profile.Managed,
			Provider: providerSnapshotFromEntity(target.Profile.Provider), AuthJSON: string(authJSON),
		}
		action := importAction{source: source, target: target, identity: identity, after: &desired}
		plan := importPlan{
			actions: []importAction{action}, resolvedNames: map[string]string{name: name},
			activeTarget: name,
		}
		if _, err := catalog.applyImport(ctx, plan); err != nil {
			return err
		}
		result = Report{Profile: desired, Active: true, Enabled: target.Enabled, AccountID: target.AccountID}
		return nil
	})
	return result, err
}

func (catalog *Catalog) selectedOpenAITargetLocked(ctx context.Context, name string) (Report, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Report{}, errors.New("selected login profile is required")
	}
	listed, err := catalog.list(ctx)
	if err != nil {
		return Report{}, err
	}
	for _, report := range listed {
		if report.Profile.Name != name {
			continue
		}
		if report.Profile.Provider.Kind != profileentity.ProviderOpenAI {
			return Report{}, fmt.Errorf(
				"profile %q uses %s; godex login --profile currently supports OpenAI/Codex profiles only",
				name, report.Profile.Provider.Kind,
			)
		}
		return report, nil
	}
	return Report{}, fmt.Errorf(profileNotFoundFormat, name)
}
