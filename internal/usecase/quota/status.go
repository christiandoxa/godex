package quota

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type accountStore interface {
	List(context.Context) ([]accountentity.Account, error)
	Current(context.Context) (accountentity.Account, error)
	Resolve(context.Context, string) (accountentity.Account, error)
	CodexHome(string) string
}

type profileSource interface {
	QuotaTargets(context.Context) ([]profilemodel.QuotaTarget, error)
}

type usageGateway interface {
	Fetch(context.Context, string) (quotamodel.Usage, error)
}

type usageSnapshotStore interface {
	Load(context.Context, string) (quotamodel.UsageSnapshot, bool, error)
	Save(context.Context, string, quotamodel.UsageSnapshot) error
}

type rawUsageGateway interface {
	FetchRaw(context.Context, string) ([]byte, error)
}

type overrideUsageGateway interface {
	FetchAt(context.Context, string, string) (quotamodel.Usage, error)
}

type policyUsageGateway interface {
	FetchAtPolicy(context.Context, string, string, bool) (quotamodel.Usage, error)
}

type rawOverrideUsageGateway interface {
	FetchRawAt(context.Context, string, string) ([]byte, error)
}

type externalProfileGateway interface {
	FetchQuota(context.Context, profilemodel.QuotaTarget) (quotamodel.ExternalInfo, error)
}

type rawExternalProfileGateway interface {
	FetchQuotaRaw(context.Context, profilemodel.QuotaTarget) ([]byte, error)
}

type modelProviderInspector interface {
	InspectModelProvider(context.Context, string) (*profilemodel.ModelProviderSetting, error)
}

type virtualGateway interface {
	Collect(context.Context, string, string) []quotamodel.VirtualResult
}

type Options struct {
	All            bool
	Selector       string
	BaseURL        string
	AuthFilter     string
	ProviderFilter string
}

type Status struct {
	accounts      accountStore
	profiles      profileSource
	usage         usageGateway
	virtual       virtualGateway
	external      map[string]externalProfileGateway
	modelProvider modelProviderInspector
	now           func() time.Time
	usageMu       sync.Mutex
	usageCache    map[string]usageSnapshot
	snapshotCache map[string]quotamodel.UsageSnapshot
	snapshots     usageSnapshotStore
	snapshotQueue *queuedUsageSnapshotStore
	probes        probeRefreshGate
	probeQueueMu  sync.Mutex
	probeQueue    *ProbeRefreshQueue
}

func NewStatus(accounts accountStore, usage usageGateway) *Status {
	return &Status{
		accounts: accounts, usage: usage,
		external: make(map[string]externalProfileGateway), now: time.Now,
	}
}

func (status *Status) SetProfiles(profiles profileSource) { status.profiles = profiles }

func (status *Status) SetUsageSnapshotStore(store usageSnapshotStore) {
	status.usageMu.Lock()
	defer status.usageMu.Unlock()
	status.snapshots = store
	status.snapshotQueue = nil
	status.snapshotCache = make(map[string]quotamodel.UsageSnapshot)
}

// SetQueuedUsageSnapshotStore moves snapshot writes off the request path.
func (status *Status) SetQueuedUsageSnapshotStore(store usageSnapshotStore) {
	status.usageMu.Lock()
	defer status.usageMu.Unlock()
	if store == nil {
		status.snapshots = nil
		status.snapshotQueue = nil
		status.snapshotCache = make(map[string]quotamodel.UsageSnapshot)
		return
	}
	queued := newQueuedUsageSnapshotStore(store)
	status.snapshots = queued
	status.snapshotQueue = queued
	status.snapshotCache = make(map[string]quotamodel.UsageSnapshot)
}

// ShutdownUsageSnapshotSave drains the process-lifetime snapshot writer.
func (status *Status) ShutdownUsageSnapshotSave(ctx context.Context) error {
	status.usageMu.Lock()
	queue := status.snapshotQueue
	status.usageMu.Unlock()
	if queue == nil {
		return nil
	}
	return queue.shutdown(ctx)
}

// Close stops status-owned background work before process exit.
func (status *Status) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = status.ShutdownProbeRefresh(ctx)
	_ = status.ShutdownUsageSnapshotSave(ctx)
}

func (status *Status) SetVirtual(virtual virtualGateway) { status.virtual = virtual }

func (status *Status) SetExternalProvider(provider string, gateway externalProfileGateway) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || gateway == nil {
		return
	}
	if status.external == nil {
		status.external = make(map[string]externalProfileGateway)
	}
	status.external[provider] = gateway
}

func (status *Status) SetModelProviderInspector(inspector modelProviderInspector) {
	status.modelProvider = inspector
}

func (status *Status) DoctorReports(ctx context.Context) ([]quotamodel.Report, error) {
	return status.Run(ctx, Options{All: true})
}

func (status *Status) Raw(ctx context.Context, selector, baseURL string) ([]byte, error) {
	if status.profiles != nil {
		target, err := status.selectedProfile(ctx, selector)
		if err != nil {
			return nil, err
		}
		inspected := status.inspectProfileQuotaTarget(ctx, target)
		if inspected.modelProviderErr != nil {
			return nil, inspected.modelProviderErr
		}
		if inspected.modelProvider != nil {
			return codexModelProviderQuotaJSON(*inspected.modelProvider)
		}
		if strings.EqualFold(strings.TrimSpace(target.Auth), "no-auth") {
			return nil, noAuthQuotaError(target)
		}
		if target.Provider != "openai" {
			gateway := status.externalProvider(target.Provider)
			if gateway == nil {
				return nil, errors.New("raw quota is not supported for this profile provider")
			}
			if raw, ok := gateway.(rawExternalProfileGateway); ok {
				return raw.FetchQuotaRaw(ctx, target)
			}
			info, err := gateway.FetchQuota(ctx, target)
			if err != nil {
				return nil, err
			}
			return marshalExternalQuotaJSON(info)
		}
		if !target.Compatible {
			return nil, errors.New("raw quota requires a quota-compatible OpenAI profile")
		}
		return status.fetchRaw(ctx, target.CodexHome, baseURL)
	}
	account, err := status.selectedAccount(ctx, selector)
	if err != nil {
		return nil, err
	}
	return status.fetchRaw(ctx, status.accounts.CodexHome(account.ID), baseURL)
}

func (status *Status) fetchRaw(ctx context.Context, home, baseURL string) ([]byte, error) {
	if baseURL != "" {
		raw, ok := status.usage.(rawOverrideUsageGateway)
		if !ok {
			return nil, errors.New("quota base URL override is not supported")
		}
		return raw.FetchRawAt(ctx, home, baseURL)
	}
	raw, ok := status.usage.(rawUsageGateway)
	if !ok {
		return nil, errors.New("raw quota output is not supported")
	}
	return raw.FetchRaw(ctx, home)
}

func (status *Status) Ready(ctx context.Context, account accountentity.Account) (bool, error) {
	availability, err := status.Availability(ctx, account)
	return availability.Ready, err
}

func (status *Status) Run(ctx context.Context, options Options) ([]quotamodel.Report, error) {
	if status.profiles != nil {
		return status.runProfiles(ctx, options)
	}
	accounts, err := status.selectedAccounts(ctx, options)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, errors.New("no managed accounts; run `godex login` or `godex profile import-current`")
	}
	current, _ := status.accounts.Current(ctx)
	reports := make([]quotamodel.Report, 0, len(accounts))
	for _, account := range accounts {
		report := quotamodel.Report{
			AccountName: account.Name,
			Email:       account.Email,
			Active:      account.ID == current.ID,
			Enabled:     account.Enabled,
		}
		if account.Enabled {
			report.Usage, report.Err = status.fetchUsage(ctx, account, options.BaseURL)
		}
		report.State = quotaState(report, status.now())
		reports = append(reports, report)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return reports, nil
}

func (status *Status) selectedAccount(ctx context.Context, selector string) (accountentity.Account, error) {
	if selector != "" {
		return status.accounts.Resolve(ctx, selector)
	}
	return status.accounts.Current(ctx)
}

func (status *Status) fetchHomeUsage(ctx context.Context, home, baseURL string) (quotamodel.Usage, error) {
	return status.fetchProbe(ctx, func() (quotamodel.Usage, error) {
		if baseURL == "" {
			return status.usage.Fetch(ctx, home)
		}
		override, ok := status.usage.(overrideUsageGateway)
		if !ok {
			return quotamodel.Usage{}, errors.New("quota base URL override is not supported")
		}
		return override.FetchAt(ctx, home, baseURL)
	})
}

func (status *Status) fetchUsage(ctx context.Context, account accountentity.Account, baseURL string) (quotamodel.Usage, error) {
	return status.fetchHomeUsage(ctx, status.accounts.CodexHome(account.ID), baseURL)
}

func (status *Status) selectedAccounts(ctx context.Context, options Options) ([]accountentity.Account, error) {
	if options.All {
		if options.Selector != "" {
			return nil, errors.New("quota selector cannot be combined with --all")
		}
		return status.accounts.List(ctx)
	}
	if options.Selector != "" {
		account, err := status.accounts.Resolve(ctx, options.Selector)
		if err != nil {
			return nil, err
		}
		return []accountentity.Account{account}, nil
	}
	account, err := status.accounts.Current(ctx)
	if err != nil {
		return nil, err
	}
	return []accountentity.Account{account}, nil
}

func quotaState(report quotamodel.Report, now time.Time) string {
	if !report.Enabled {
		return "disabled"
	}
	if report.Err != nil {
		return "error"
	}
	usage := report.Usage
	if usage.Allowed != nil && !*usage.Allowed {
		return "exhausted"
	}
	if usage.LimitReached != nil && *usage.LimitReached {
		return "exhausted"
	}
	if windowExhausted(usage.Primary, now) || windowExhausted(usage.Secondary, now) {
		return "exhausted"
	}
	return "ready"
}

func windowExhausted(window *quotamodel.Window, now time.Time) bool {
	if window == nil || window.UsedPercent == nil || *window.UsedPercent < 100 {
		return false
	}
	return window.ResetAt == nil || *window.ResetAt > now.Unix()
}
