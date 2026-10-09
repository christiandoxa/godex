package quota

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestWarmupStartupProbesBoundsRefreshesAndReusesCache(t *testing.T) {
	accounts := []accountentity.Account{
		{ID: "one", Enabled: true},
	}
	usage := &countingUsage{}
	status := NewStatus(fakeAccounts{accounts: accounts}, usage)

	status.WarmupStartupProbes(context.Background(), accounts, "", false)
	if usage.calls != 1 {
		t.Fatalf("startup probe calls = %d, want 1", usage.calls)
	}
	status.WarmupStartupProbes(context.Background(), accounts, "", false)
	if usage.calls != 1 {
		t.Fatalf("second warmup calls = %d, want 1", usage.calls)
	}
	status.WarmupStartupProbes(context.Background(), accounts, "", false)
	if usage.calls != 1 {
		t.Fatalf("fresh startup cache caused %d calls, want 1", usage.calls)
	}
}

func TestWarmupStartupProbesUsesSyncThenQueuedWarmLimit(t *testing.T) {
	accounts := []accountentity.Account{
		{ID: "one", Enabled: true},
		{ID: "two", Enabled: true},
		{ID: "three", Enabled: true},
		{ID: "four", Enabled: true},
	}
	usage := &startupCountingUsage{release: make(chan struct{})}
	status := NewStatus(fakeAccounts{accounts: accounts}, usage)
	defer status.Close()
	observed := status.ProbeRefreshRevision()

	status.WarmupStartupProbes(context.Background(), accounts, "", false)
	if calls := usage.calls.Load(); calls != startupProbeSyncWarmLimit {
		t.Fatalf("synchronous startup probes = %d, want %d", calls, startupProbeSyncWarmLimit)
	}
	close(usage.release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for usage.calls.Load() < startupProbeWarmLimit {
		observed = status.ProbeRefreshRevision()
		if !status.WaitProbeRefresh(ctx, observed) {
			t.Fatal("queued startup probes did not complete")
		}
	}
	if calls := usage.calls.Load(); calls != startupProbeWarmLimit {
		t.Fatalf("total startup probes = %d, want %d", calls, startupProbeWarmLimit)
	}
}

type startupCountingUsage struct {
	calls   atomic.Int32
	release chan struct{}
}

func (usage *startupCountingUsage) Fetch(_ context.Context, home string) (quotamodel.Usage, error) {
	if home != "/managed/one" {
		<-usage.release
	}
	usage.calls.Add(1)
	return quotamodel.Usage{}, nil
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

	observed := status.ProbeRefreshRevision()
	status.WarmupStartupProbes(context.Background(), accounts, "", false)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !status.WaitProbeRefresh(ctx, observed) {
		t.Fatal("startup refresh did not complete")
	}
	if _, ok := status.cachedLiveUsage("/managed/ready", status.now()); !ok {
		t.Fatal("successful startup probe was not cached")
	}
}
