package quota

import (
	"context"
	"strings"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Prodex limits startup refresh work to three profiles and probes only the
// first one synchronously. Remaining selected profiles use the queue.
const (
	startupProbeWarmLimit     = 3
	startupProbeSyncWarmLimit = 1
)

// WarmupStartupProbes refreshes the bounded launch pool before a runtime child
// starts. Probe failures are intentionally ignored: launch-time admission owns
// fail-open behavior, while this warmup only improves the first route's cache.
func (status *Status) WarmupStartupProbes(
	ctx context.Context,
	accounts []accountentity.Account,
	baseURL string,
	noProxy bool,
) {
	targets := make([]startupProbeTarget, 0, len(accounts))
	for _, account := range accounts {
		targets = append(targets, startupProbeTarget{
			id: account.ID, home: status.accounts.CodexHome(account.ID), enabled: account.Enabled,
		})
	}
	status.warmupStartupProbeTargets(ctx, targets, baseURL, noProxy)
}

// WarmupStartupProfiles refreshes managed or external profile homes in the
// launch pool. Profile catalog entries are not necessarily account-store
// entries, so the home carried by the runtime profile is authoritative here.
func (status *Status) WarmupStartupProfiles(
	ctx context.Context,
	profiles []proxymodel.Account,
	baseURL string,
	noProxy bool,
) {
	targets := make([]startupProbeTarget, 0, len(profiles))
	for _, profile := range profiles {
		targets = append(targets, startupProbeTarget{
			id: profile.ID, home: profile.Home, enabled: profile.Enabled,
		})
	}
	status.warmupStartupProbeTargets(ctx, targets, baseURL, noProxy)
}

type startupProbeTarget struct {
	id      string
	home    string
	enabled bool
}

func (status *Status) warmupStartupProbeTargets(
	ctx context.Context,
	targets []startupProbeTarget,
	baseURL string,
	noProxy bool,
) {
	if status == nil || status.usage == nil || ctx == nil {
		return
	}
	baseURL = strings.TrimSpace(baseURL)
	refresh := make([]startupProbeTarget, 0, startupProbeWarmLimit)
	for _, target := range targets {
		if ctx.Err() != nil {
			return
		}
		if !target.enabled || strings.TrimSpace(target.id) == "" || strings.TrimSpace(target.home) == "" {
			continue
		}
		if _, _, ok := status.cachedUsageWithSource(ctx, target.id, target.home, status.now(), nil); ok {
			continue
		}
		refresh = append(refresh, target)
		if len(refresh) == startupProbeWarmLimit {
			break
		}
	}
	for index, target := range refresh {
		if ctx.Err() != nil {
			return
		}
		if index < startupProbeSyncWarmLimit {
			_, _ = status.availabilityAtPolicyForHome(ctx, target.id, target.home, baseURL, noProxy)
			continue
		}
		_ = status.scheduleProbeRefresh(ctx, target.id, target.home, baseURL, noProxy)
	}
}
