package profile

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const profileNotFoundFormat = "profile %q does not exist"

type repository interface {
	ManagedHome(string) string
	List(context.Context) ([]profileentity.Profile, error)
	Current(context.Context) (profileentity.Profile, error)
	HasActive(context.Context) (bool, error)
	Resolve(context.Context, string) (profileentity.Profile, error)
	Create(context.Context, profileentity.Profile, string, bool, bool) error
	SetActive(context.Context, string) (profileentity.Profile, error)
	ClearActive(context.Context) error
	Remove(context.Context, string, bool) (profileentity.Profile, error)
	Acquire(context.Context, string) (func() error, error)
	ReadAuthJSON(string) ([]byte, error)
	ImportOpenAI(context.Context, profileentity.Profile, []byte, bool) error
	ReplaceAuth(context.Context, string, []byte) error
	EncodeBundle(profilemodel.BundlePayload, string) ([]byte, error)
	DecodeBundle([]byte, string) (profilemodel.BundlePayload, bool, error)
	WriteBundle(string, []byte) error
	ReadBundle(string) ([]byte, error)
	ImportProvider(context.Context, profileentity.Profile, map[string]string, bool) error
	ReplaceProvider(context.Context, string, string, profileentity.Provider, map[string]string, bool) error
	ReadProviderSecret(string, string) (string, error)
}

type accountStore interface {
	List(context.Context) ([]accountentity.Account, error)
	Current(context.Context) (accountentity.Account, error)
	SetActive(context.Context, string) (accountentity.Account, error)
	RemoveProfile(context.Context, string, bool) (accountentity.Account, error)
	ReplaceImportedAuth(context.Context, string, []byte) error
	CodexHome(string) string
}

type Report struct {
	Profile   profileentity.Profile
	Active    bool
	Enabled   bool
	AccountID string
}

type authInspector interface {
	InspectAuthJSON(context.Context, []byte) (accountentity.Identity, error)
}

type quotaAuthInspector interface {
	InspectQuotaAuth(context.Context, string) (profilemodel.QuotaAuthSummary, error)
}

type claudeSource interface {
	Load(context.Context) (profilemodel.BuiltinCredential, error)
}

type Catalog struct {
	profiles         repository
	accounts         accountStore
	auth             authInspector
	quotaAuth        quotaAuthInspector
	claude           claudeSource
	currentCodexHome string
}

func NewCatalog(profiles repository, accounts accountStore, currentCodexHome string) *Catalog {
	return &Catalog{profiles: profiles, accounts: accounts, currentCodexHome: currentCodexHome}
}

func (catalog *Catalog) SetClaudeSource(source claudeSource) { catalog.claude = source }

func (catalog *Catalog) SetAuthInspector(inspector authInspector) {
	catalog.auth = inspector
	if quotaInspector, ok := inspector.(quotaAuthInspector); ok {
		catalog.quotaAuth = quotaInspector
	}
}

func (catalog *Catalog) List(ctx context.Context) ([]Report, error) {
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

func (catalog *Catalog) Add(ctx context.Context, request profilemodel.AddRequest) (Report, error) {
	if err := profileentity.ValidateName(request.Name); err != nil {
		return Report{}, err
	}
	sourceKind, err := profileentity.ResolveSource(request.CodexHome, request.CopyFrom, request.CopyCurrent)
	if err != nil {
		return Report{}, err
	}
	if err := catalog.ensureNameAvailable(ctx, request.Name); err != nil {
		return Report{}, err
	}
	value, source, err := catalog.planProfile(request, sourceKind)
	if err != nil {
		return Report{}, err
	}
	if err := catalog.ensureHomeAvailable(ctx, value.CodexHome); err != nil {
		return Report{}, err
	}
	activeExists := false
	if _, currentErr := catalog.Current(ctx); currentErr == nil {
		activeExists = true
	}
	activate := profileentity.ShouldActivate(activeExists, request.Activate)
	if err := catalog.profiles.Create(ctx, value, source, request.Insecure, activate); err != nil {
		return Report{}, err
	}
	return Report{Profile: value, Active: activate, Enabled: true}, nil
}

func (catalog *Catalog) Use(ctx context.Context, name string) (Report, error) {
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

func (catalog *Catalog) Remove(ctx context.Context, request profilemodel.RemoveRequest) ([]Report, error) {
	listed, err := catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	targets, err := removalTargets(listed, request)
	if err != nil {
		return nil, err
	}
	if err := validateDeleteTargets(targets, request.DeleteHome); err != nil {
		return nil, err
	}
	removed := make([]Report, 0, len(targets))
	for _, target := range targets {
		report, err := catalog.removeTarget(ctx, target, request.DeleteHome)
		if err != nil {
			return removed, err
		}
		removed = append(removed, report)
	}
	return removed, nil
}

func validateDeleteTargets(targets []Report, deleteHome bool) error {
	if !deleteHome {
		return nil
	}
	for _, target := range targets {
		if target.AccountID == "" && !target.Profile.Managed {
			return fmt.Errorf("refusing to delete external path %s", target.Profile.CodexHome)
		}
	}
	return nil
}

func (catalog *Catalog) removeTarget(ctx context.Context, target Report, deleteHome bool) (Report, error) {
	if target.AccountID != "" {
		account, err := catalog.accounts.RemoveProfile(ctx, target.AccountID, deleteHome)
		if err != nil {
			return Report{}, err
		}
		return Report{Profile: accountProfile(account, target.Profile.CodexHome), AccountID: account.ID}, nil
	}
	profile, err := catalog.profiles.Remove(ctx, target.Profile.Name, deleteHome)
	if err != nil {
		return Report{}, err
	}
	return Report{Profile: profile}, nil
}

func (catalog *Catalog) Summary(ctx context.Context) (profilemodel.Summary, error) {
	listed, err := catalog.List(ctx)
	if err != nil {
		return profilemodel.Summary{}, err
	}
	summary := profilemodel.Summary{Count: len(listed)}
	for _, report := range listed {
		if report.Active {
			summary.Active = report.Profile.Name
			break
		}
	}
	return summary, nil
}

func (catalog *Catalog) planProfile(request profilemodel.AddRequest, kind profileentity.SourceKind) (profileentity.Profile, string, error) {
	value := profileentity.Profile{Name: request.Name, Managed: kind != profileentity.SourceExternalHome, Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI}}
	source := ""
	switch kind {
	case profileentity.SourceExternalHome:
		absolute, err := filepath.Abs(request.CodexHome)
		if err != nil {
			return value, "", err
		}
		value.CodexHome = filepath.Clean(absolute)
	case profileentity.SourceCopyFrom:
		absolute, err := filepath.Abs(request.CopyFrom)
		if err != nil {
			return value, "", err
		}
		source = filepath.Clean(absolute)
		value.CodexHome = catalog.profiles.ManagedHome(request.Name)
	case profileentity.SourceCopyCurrent:
		source = catalog.currentCodexHome
		value.CodexHome = catalog.profiles.ManagedHome(request.Name)
	case profileentity.SourceEmptyManaged:
		value.CodexHome = catalog.profiles.ManagedHome(request.Name)
	}
	return value, source, profileentity.Validate(value)
}

func (catalog *Catalog) ensureHomeAvailable(ctx context.Context, home string) error {
	listed, err := catalog.List(ctx)
	if err != nil {
		return err
	}
	want := filepath.Clean(home)
	for _, current := range listed {
		if filepath.Clean(current.Profile.CodexHome) == want {
			return fmt.Errorf("CODEX_HOME %s is already registered", home)
		}
	}
	return nil
}

func (catalog *Catalog) ensureNameAvailable(ctx context.Context, name string) error {
	listed, err := catalog.List(ctx)
	if err != nil {
		return err
	}
	for _, current := range listed {
		if current.Profile.Name == name {
			return fmt.Errorf("profile %q already exists", name)
		}
	}
	return nil
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

func removalTargets(listed []Report, request profilemodel.RemoveRequest) ([]Report, error) {
	if request.All {
		if request.Name != "" {
			return nil, errors.New("profile name cannot be combined with --all")
		}
		return listed, nil
	}
	if request.Name == "" {
		return nil, errors.New("provide a profile name or pass --all")
	}
	for _, current := range listed {
		if current.Profile.Name == request.Name {
			return []Report{current}, nil
		}
	}
	return nil, fmt.Errorf(profileNotFoundFormat, request.Name)
}

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
	return profilemodel.LaunchTarget{Name: profile.Name, CodexHome: profile.CodexHome, Provider: string(profile.Provider.Kind)}, true, nil
}
