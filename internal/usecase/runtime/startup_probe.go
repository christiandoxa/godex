package runtime

import (
	"context"
	"strings"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

// quotaStartupWarmup is an optional quota capability used to refresh the
// launch pool before the child Codex process starts. It is deliberately
// separate from quotaPreflight: startup probes are best effort and must not
// turn a skipped admission check into a launch failure.
type quotaStartupWarmup interface {
	WarmupStartupProfiles(context.Context, []proxyconfig.Account, string, bool)
}

func (runner *Runner) warmupStartupProbes(
	ctx context.Context,
	providerKind string,
	profiles []proxyconfig.Account,
	upstream string,
	noProxy bool,
) {
	if runner == nil || strings.TrimSpace(providerKind) != "" || runner.quota == nil {
		return
	}
	warmup, ok := runner.quota.(quotaStartupWarmup)
	if !ok || len(profiles) == 0 {
		return
	}
	warmup.WarmupStartupProfiles(ctx, profiles, upstream, noProxy)
}
