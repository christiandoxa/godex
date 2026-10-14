package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	accountmodel "github.com/christiandoxa/godex/internal/model/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const (
	doctorEventLimit     = 4096
	doctorBundleMaxBytes = 4 << 20
)

type doctorAccounts interface {
	Prepare() error
	Root() string
	List(context.Context) ([]accountentity.Account, error)
}

type versionedCodex interface {
	Version(context.Context) (string, error)
	CheckProxySupport(context.Context) error
}

type doctorSessionIndexRepairer interface {
	RepairSessionIndex(context.Context, string, string, string) error
}

type activeCodexHomeResolver interface {
	CurrentCodexHome(context.Context) (string, error)
}

type doctorActivity interface {
	Overview(context.Context) (runtimemodel.Overview, error)
	Events(context.Context, int) ([]runtimemodel.Event, error)
}

type doctorRouteHealthStateReader interface {
	LoadRouteHealth(context.Context, time.Time) ([]routingentity.RouteHealthScore, error)
	LoadRouteMemory(context.Context, time.Time) ([]routingentity.RouteMemoryScore, error)
	LoadRouteCircuits(context.Context, time.Time, []routingentity.RouteHealthScore) ([]routingentity.RouteCircuit, error)
	LoadTransportBackoffs(context.Context, time.Time) ([]routingentity.TransportBackoff, error)
}

type doctorQuota interface {
	DoctorReports(context.Context) ([]quotamodel.Report, error)
}

type doctorBundleStore interface {
	Write(string, []byte) (string, error)
}

type doctorImportJournalRepairer interface {
	CountImportAuthJournals(context.Context) (int, error)
	RepairImportAuthJournals(context.Context) (int, error)
}

type Doctor struct {
	accounts        doctorAccounts
	codex           versionedCodex
	repairer        doctorSessionIndexRepairer
	activeHome      activeCodexHomeResolver
	sharedCodexHome string
	activity        doctorActivity
	routeState      doctorRouteHealthStateReader
	quota           doctorQuota
	bundles         doctorBundleStore
	imports         doctorImportJournalRepairer
	now             func() time.Time
}

func NewDoctor(accounts doctorAccounts, codex versionedCodex) *Doctor {
	return &Doctor{accounts: accounts, codex: codex, now: time.Now}
}

func (doctor *Doctor) SetActivity(activity doctorActivity) { doctor.activity = activity }

func (doctor *Doctor) SetRouteHealthState(state doctorRouteHealthStateReader) {
	doctor.routeState = state
}

func (doctor *Doctor) SetSessionIndexRepairer(repairer doctorSessionIndexRepairer) {
	doctor.repairer = repairer
}

func (doctor *Doctor) SetActiveCodexHomeResolver(resolver activeCodexHomeResolver) {
	doctor.activeHome = resolver
}

func (doctor *Doctor) SetSharedCodexHome(home string) { doctor.sharedCodexHome = home }

// RepairSessionIndex asks Codex to reconcile sessions for the active home.
func (doctor *Doctor) RepairSessionIndex(ctx context.Context) error {
	if doctor.repairer == nil {
		return fmt.Errorf("session index repair is not configured")
	}
	if doctor.activeHome == nil {
		return fmt.Errorf("active Codex home resolution is not configured")
	}
	codexHome, err := doctor.activeHome.CurrentCodexHome(ctx)
	if err != nil {
		return err
	}
	return doctor.repairer.RepairSessionIndex(ctx, codexHome, doctor.sharedCodexHome, doctor.accounts.Root())
}

func (doctor *Doctor) SetQuota(quota doctorQuota) { doctor.quota = quota }

func (doctor *Doctor) SetBundleStore(store doctorBundleStore) { doctor.bundles = store }

func (doctor *Doctor) SetImportJournalRepairer(repairer doctorImportJournalRepairer) {
	doctor.imports = repairer
}

func (doctor *Doctor) SaveBundle(path string, content []byte) (string, error) {
	if doctor == nil || doctor.bundles == nil {
		return "", fmt.Errorf("doctor bundle storage is not configured")
	}
	if len(content) > doctorBundleMaxBytes {
		return "", fmt.Errorf("doctor bundle exceeds safe size limit (%d bytes)", doctorBundleMaxBytes)
	}
	return doctor.bundles.Write(path, content)
}

func (doctor *Doctor) Run(ctx context.Context) (accountmodel.DoctorReport, error) {
	if err := doctor.accounts.Prepare(); err != nil {
		return accountmodel.DoctorReport{}, err
	}
	if err := doctor.codex.CheckProxySupport(ctx); err != nil {
		return accountmodel.DoctorReport{}, err
	}
	codexVersion, err := doctor.codex.Version(ctx)
	if err != nil {
		return accountmodel.DoctorReport{}, err
	}
	accounts, err := doctor.accounts.List(ctx)
	if err != nil {
		return accountmodel.DoctorReport{}, err
	}
	enabled := 0
	for _, account := range accounts {
		if account.Enabled {
			enabled++
		}
	}
	return accountmodel.DoctorReport{
		GodexHome:    doctor.accounts.Root(),
		CodexVersion: codexVersion,
		AccountCount: len(accounts),
		EnabledCount: enabled,
	}, nil
}

func (doctor *Doctor) Diagnose(ctx context.Context, options runtimemodel.DoctorOptions) (runtimemodel.DoctorDiagnostics, error) {
	var importStatus *runtimemodel.DoctorImportAuthJournals
	if doctor.imports != nil {
		repaired := 0
		if options.RepairImportAuthJournals {
			var err error
			repaired, err = doctor.imports.RepairImportAuthJournals(ctx)
			if err != nil {
				return runtimemodel.DoctorDiagnostics{}, err
			}
		}
		orphanCount, err := doctor.imports.CountImportAuthJournals(ctx)
		if err != nil {
			return runtimemodel.DoctorDiagnostics{}, err
		}
		status := "ok"
		if orphanCount > 0 {
			status = "warning"
		}
		importStatus = &runtimemodel.DoctorImportAuthJournals{
			OrphanCount: orphanCount, RepairPerformed: options.RepairImportAuthJournals,
			Repaired: repaired, Status: status,
		}
	} else if options.RepairImportAuthJournals {
		return runtimemodel.DoctorDiagnostics{}, fmt.Errorf("profile import journal repair is not configured")
	}
	baseline, err := doctor.Run(ctx)
	if err != nil {
		return runtimemodel.DoctorDiagnostics{}, err
	}
	report := runtimemodel.DoctorDiagnostics{
		GeneratedAt: doctor.now().UTC().Format(time.RFC3339),
		GodexHome:   baseline.GodexHome, CodexVersion: baseline.CodexVersion,
		AccountCount: baseline.AccountCount, EnabledCount: baseline.EnabledCount,
		ImportAuthJournals: importStatus,
	}
	if options.Install {
		report.Install = doctorInstallChecks(baseline)
	}
	if options.Runtime {
		runtimeReport, err := doctor.runtimeDiagnostics(ctx, options.TailBytes)
		if err != nil {
			return runtimemodel.DoctorDiagnostics{}, err
		}
		if options.SuggestPolicy {
			suggestions := doctorPolicySuggestions(runtimeReport.Events)
			count := len(suggestions)
			runtimeReport.PolicySuggestionCount = &count
			runtimeReport.PolicySuggestions = &suggestions
		}
		report.Runtime = &runtimeReport
	}
	if options.Quota {
		quota, err := doctor.quotaDiagnostics(ctx)
		if err != nil {
			return runtimemodel.DoctorDiagnostics{}, err
		}
		report.Quota = quota
	}
	return report, nil
}

func doctorInstallChecks(baseline accountmodel.DoctorReport) []runtimemodel.DoctorCheck {
	return []runtimemodel.DoctorCheck{
		{Name: "Codex CLI", Status: "ok (" + baseline.CodexVersion + ")"},
		{Name: "Managed proxy config", Status: "ready"},
		{Name: "Godex home", Status: baseline.GodexHome},
	}
}

func (doctor *Doctor) runtimeDiagnostics(ctx context.Context, tailBytes int) (runtimemodel.DoctorRuntime, error) {
	if doctor.activity == nil {
		return runtimemodel.DoctorRuntime{}, fmt.Errorf("runtime diagnostics are not configured")
	}
	overview, err := doctor.activity.Overview(ctx)
	if err != nil {
		return runtimemodel.DoctorRuntime{}, err
	}
	events, err := doctor.activity.Events(ctx, doctorEventLimit)
	if err != nil {
		return runtimemodel.DoctorRuntime{}, err
	}
	var routeState *doctorRouteHealthState
	if doctor.routeState != nil {
		now := doctor.now()
		health, err := doctor.routeState.LoadRouteHealth(ctx, now)
		if err != nil {
			return runtimemodel.DoctorRuntime{}, fmt.Errorf("load route health: %w", err)
		}
		memory, err := doctor.routeState.LoadRouteMemory(ctx, now)
		if err != nil {
			return runtimemodel.DoctorRuntime{}, fmt.Errorf("load route memory: %w", err)
		}
		// A doctor read must not reset a timed-out circuit or reserve a
		// probe. Prefer the non-mutating snapshot reader when available.
		var circuits []routingentity.RouteCircuit
		if inspector, ok := doctor.routeState.(interface {
			InspectRouteCircuits(context.Context, time.Time) ([]routingentity.RouteCircuit, error)
		}); ok {
			circuits, err = inspector.InspectRouteCircuits(ctx, now)
		} else {
			circuits, err = doctor.routeState.LoadRouteCircuits(ctx, now, health)
		}
		if err != nil {
			return runtimemodel.DoctorRuntime{}, fmt.Errorf("inspect route circuits: %w", err)
		}
		backoffs, err := doctor.routeState.LoadTransportBackoffs(ctx, now)
		if err != nil {
			return runtimemodel.DoctorRuntime{}, fmt.Errorf("load transport backoffs: %w", err)
		}
		routeState = &doctorRouteHealthState{now: now, health: health, memory: memory, circuits: circuits, backoffs: backoffs}
	}
	return runtimemodel.DoctorRuntime{
		Overview: overview, Events: doctorTailEvents(events, tailBytes), RouteHealth: doctorRouteHealthWithState(events, routeState), TailBytes: tailBytes,
	}, nil
}

func doctorTailEvents(events []runtimemodel.Event, maxBytes int) []runtimemodel.Event {
	if maxBytes <= 0 || len(events) == 0 {
		return nil
	}
	start := len(events)
	used := 0
	for index := len(events) - 1; index >= 0; index-- {
		encoded, err := json.Marshal(events[index])
		if err != nil {
			continue
		}
		lineBytes := len(encoded) + 1
		if used+lineBytes > maxBytes {
			break
		}
		used += lineBytes
		start = index
	}
	if start == len(events) {
		return nil
	}
	result := append([]runtimemodel.Event(nil), events[start:]...)
	for index := range result {
		result[index] = normalizeDoctorEvent04358(result[index])
		result[index].AccountID = ""
	}
	return result
}

func (doctor *Doctor) quotaDiagnostics(ctx context.Context) ([]runtimemodel.DoctorQuota, error) {
	if doctor.quota == nil {
		return nil, fmt.Errorf("quota diagnostics are not configured")
	}
	reports, err := doctor.quota.DoctorReports(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]runtimemodel.DoctorQuota, 0, len(reports))
	for _, report := range reports {
		name := report.ProfileName
		if name == "" {
			name = report.AccountName
		}
		provider := report.Provider
		if provider == "" {
			provider = "openai"
		}
		state := report.State
		if report.Err != nil {
			state = "error"
		}
		quota := runtimemodel.DoctorQuota{
			Profile: name, Provider: provider, Auth: report.Auth, State: state,
			Plan: report.Usage.PlanType, FiveHour: doctorWindow(report.Usage.Primary),
			Weekly: doctorWindow(report.Usage.Secondary), Active: report.Active, Enabled: report.Enabled,
		}
		if report.Err != nil {
			quota.Error = doctorQuotaErrorSummary(report.Err)
		} else if report.External != nil {
			quota.External = &runtimemodel.DoctorExternalQuota{
				Status: report.External.Status, Main: report.External.Main, Reset: report.External.Reset,
			}
		} else if provider == "openai" {
			quota.OpenAI = doctorOpenAIQuota(report.Usage)
		}
		result = append(result, quota)
	}
	return result, nil
}

func doctorWindow(window *quotamodel.Window) string {
	if window == nil || window.UsedPercent == nil {
		return "-"
	}
	remaining := 100 - *window.UsedPercent
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	return fmt.Sprintf("%d%%", remaining)
}
