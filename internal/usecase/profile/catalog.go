package profile

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
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
	ImportBundleProfile(context.Context, profileentity.Profile, map[string][]byte, string) error
	ReplaceAuth(context.Context, string, []byte) error
	LoginOpenAIAPIKey(context.Context, profileentity.Profile, []byte, *string, bool, bool) (profileentity.Profile, bool, error)
	ApplySelectedLoginFiles(context.Context, string, []profilemodel.ExportedSecretFile, []string) error
	ReadOpenAICompatibleBaseURL(string) (string, bool, error)
	EncodeBundle(profilemodel.BundlePayload, string) ([]byte, error)
	DecodeBundle([]byte, string) (profilemodel.BundlePayload, bool, error)
	WriteBundle(string, []byte) error
	ReadBundle(string) ([]byte, error)
	ImportProvider(context.Context, profileentity.Profile, map[string]string, bool) error
	ReplaceProvider(context.Context, string, string, profileentity.Provider, map[string]string, bool) error
	ReadProviderSecret(string, string) (string, error)
	ReadOptionalProviderSecret(string, string) (string, bool, error)
	RestoreProvider(context.Context, string, string, profileentity.Provider, map[string]*string) error
	AcquireBundleImportLock(context.Context) (func() error, error)
	WriteBundleImportJournal(profilemodel.ImportLifecycleJournal) error
	BundleImportJournals() ([]profilemodel.ImportLifecycleJournal, error)
	RemoveBundleImportJournal(string) error
	PrepareBundleImportRollback(context.Context, string, string, []string) error
	RestoreBundleImportRollback(context.Context, string, string, profilemodel.ImportLifecycleProfile) error
	CleanupBundleImportRollback(context.Context, string, string) error
	RemoveBundleImportedProfile(context.Context, profilemodel.ImportLifecycleAction, string) error
	CleanupBundleImportOwnerMarker(context.Context, string, string) error
	CheckBundleImportHomeAvailable(context.Context, string) error
	CleanupOrphanedImportStagingHomes(context.Context) error
}

type accountStore interface {
	List(context.Context) ([]accountentity.Account, error)
	Current(context.Context) (accountentity.Account, error)
	ActiveID(context.Context) (string, error)
	SetActive(context.Context, string) (accountentity.Account, error)
	ClearActive(context.Context) error
	RemoveProfile(context.Context, string, bool) (accountentity.Account, error)
	ReplaceImportedAuth(context.Context, string, []byte) error
	PrepareImportedAuthRollback(context.Context, string, string) error
	RestoreImportedAuthRollback(context.Context, string, string) error
	CleanupImportedAuthRollback(context.Context, string, string) error
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
	InspectCredential(context.Context, string) (profilemodel.BuiltinCredential, error)
}

type kiroInspector interface {
	InspectAuthSecret(context.Context, string) (profilemodel.BuiltinCredential, error)
	ValidateModelCatalog(context.Context, string) error
}

type kiroSource interface {
	Load(context.Context) (profilemodel.BuiltinCredential, error)
}

type copilotSource interface {
	Load(context.Context) (profilemodel.BuiltinCredential, error)
}

type Catalog struct {
	profiles         repository
	accounts         accountStore
	auth             authInspector
	quotaAuth        quotaAuthInspector
	claude           claudeSource
	kiro             kiroInspector
	kiroImport       kiroSource
	copilot          copilotSource
	currentCodexHome string
}

func NewCatalog(profiles repository, accounts accountStore, currentCodexHome string) *Catalog {
	return &Catalog{profiles: profiles, accounts: accounts, currentCodexHome: currentCodexHome}
}

func (catalog *Catalog) SetClaudeSource(source claudeSource) { catalog.claude = source }

func (catalog *Catalog) SetKiroInspector(inspector kiroInspector) { catalog.kiro = inspector }

func (catalog *Catalog) SetKiroSource(source kiroSource) { catalog.kiroImport = source }

func (catalog *Catalog) SetCopilotSource(source copilotSource) { catalog.copilot = source }

func (catalog *Catalog) SessionProfiles(ctx context.Context) ([]sessionmodel.ProfileHome, error) {
	reports, err := catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	homes := make([]sessionmodel.ProfileHome, 0, len(reports))
	for _, report := range reports {
		homes = append(homes, sessionmodel.ProfileHome{
			Name: report.Profile.Name, AccountID: report.AccountID, Email: report.Profile.Email,
			CodexHome: report.Profile.CodexHome, Provider: string(report.Profile.Provider.Kind), Enabled: report.Enabled,
			RoutingIDs: catalog.sessionRoutingIDs(ctx, report),
		})
	}
	return homes, nil
}

func (catalog *Catalog) sessionRoutingIDs(ctx context.Context, report Report) []string {
	home := report.Profile.CodexHome
	kind := string(report.Profile.Provider.Kind)
	if kind == "openai" {
		_, compatible, _ := catalog.profiles.ReadOpenAICompatibleBaseURL(home)
		authLabel := ""
		if catalog.quotaAuth != nil {
			if auth, err := catalog.quotaAuth.InspectQuotaAuth(ctx, home); err == nil {
				authLabel = auth.Label
			}
		}
		if compatible {
			return []string{profilemodel.RoutingID("openai-compatible:" + home)}
		}
		if authLabel == "api-key" {
			return nil
		}
		if report.AccountID != "" {
			return []string{report.AccountID}
		}
		return []string{profilemodel.RoutingID(home)}
	}
	if kind == "" {
		return nil
	}
	return []string{profilemodel.RoutingID(kind + ":" + home)}
}

func (catalog *Catalog) SetAuthInspector(inspector authInspector) {
	catalog.auth = inspector
	if quotaInspector, ok := inspector.(quotaAuthInspector); ok {
		catalog.quotaAuth = quotaInspector
	}
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
