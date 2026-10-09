package quota

import (
	"context"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestWarmupStartupProbesBoundsRefreshesAndReusesCache(t *testing.T) {
	accounts := []accountentity.Account{
		{ID: "one", Enabled: true},
		{ID: "two", Enabled: true},
		{ID: "three", Enabled: true},
		{ID: "four", Enabled: true},
	}
	usage := &countingUsage{}
	status := NewStatus(fakeAccounts{accounts: accounts}, usage)

	status.WarmupStartupProbes(context.Background(), accounts, "", false)
	if usage.calls != startupProbeWarmLimit {
		t.Fatalf("startup probe calls = %d, want %d", usage.calls, startupProbeWarmLimit)
	}
	status.WarmupStartupProbes(context.Background(), accounts, "", false)
	if usage.calls != startupProbeWarmLimit+1 {
		t.Fatalf("second warmup calls = %d, want %d", usage.calls, startupProbeWarmLimit+1)
	}
	status.WarmupStartupProbes(context.Background(), accounts, "", false)
	if usage.calls != startupProbeWarmLimit+1 {
		t.Fatalf("fresh startup cache caused %d calls, want %d", usage.calls, startupProbeWarmLimit+1)
	}
}

func TestWarmupStartupProbesIgnoresDisabledAccountsAndProbeFailures(t *testing.T) {
	accounts := []accountentity.Account{
		{ID: "disabled", Enabled: false},
		{ID: "missing", Enabled: true},
		{ID: "ready", Enabled: true},
	}
	status := NewStatus(fakeAccounts{accounts: accounts}, fakeUsage{byHome: map[string]quotamodel.Usage{
		"/managed/ready": {},
	}})

	status.WarmupStartupProbes(context.Background(), accounts, "", false)
	if _, ok := status.cachedLiveUsage("/managed/ready", status.now()); !ok {
		t.Fatal("successful startup probe was not cached")
	}
}
