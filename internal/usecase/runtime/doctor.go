package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
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

type doctorActivity interface {
	Overview(context.Context) (runtimemodel.Overview, error)
	Events(context.Context, int) ([]runtimemodel.Event, error)
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
	accounts doctorAccounts
	codex    versionedCodex
	activity doctorActivity
	quota    doctorQuota
	bundles  doctorBundleStore
	imports  doctorImportJournalRepairer
	now      func() time.Time
}

func NewDoctor(accounts doctorAccounts, codex versionedCodex) *Doctor {
	return &Doctor{accounts: accounts, codex: codex, now: time.Now}
}

func (doctor *Doctor) SetActivity(activity doctorActivity) { doctor.activity = activity }

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
	return runtimemodel.DoctorRuntime{
		Overview: overview, Events: doctorTailEvents(events, tailBytes), TailBytes: tailBytes,
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
		if report.External != nil {
			quota.External = &runtimemodel.DoctorExternalQuota{
				Status: report.External.Status, Main: report.External.Main, Reset: report.External.Reset,
			}
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
