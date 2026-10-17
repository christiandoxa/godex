package main

import (
	"testing"
	"time"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04382RouterReceivesLaunchOwnedPrecommitWatchdog(t *testing.T) {
	for _, budget := range []time.Duration{0, 30 * time.Second, 6 * time.Minute} {
		config := runtimeRoutingConfig(proxyconfig.Config{PrecommitWatchdogBudget: budget}, runtimeRouterDependencies{}, nil)
		if config.PrecommitWatchdogBudget != budget {
			t.Fatalf("Router precommit watchdog=%s, want=%s", config.PrecommitWatchdogBudget, budget)
		}
	}
}
