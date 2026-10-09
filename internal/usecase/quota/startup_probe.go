package quota

import (
	"context"
	"strings"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

const startupProbeWarmLimit = 3

// WarmupStartupProbes refreshes the bounded launch pool before a runtime child
// starts. Probe failures are intentionally ignored: launch-time admission owns
// fail-open behavior, while this warmup only improves the first route's cache.
func (status *Status) WarmupStartupProbes(
	ctx context.Context,
	accounts []accountentity.Account,
	baseURL string,
	noProxy bool,
) {
	if status == nil || status.usage == nil || ctx == nil {
		return
	}
	baseURL = strings.TrimSpace(baseURL)
	refreshed := 0
	for _, account := range accounts {
		if refreshed >= startupProbeWarmLimit || ctx.Err() != nil {
			return
		}
		if !account.Enabled || strings.TrimSpace(account.ID) == "" {
			continue
		}
		home := status.accounts.CodexHome(account.ID)
		if baseURL == "" && !noProxy {
			if _, _, ok := status.cachedUsageWithSource(ctx, account.ID, home, status.now(), nil); ok {
				continue
			}
		}
		_, _ = status.AvailabilityAtPolicy(ctx, account, baseURL, noProxy)
		refreshed++
	}
}
