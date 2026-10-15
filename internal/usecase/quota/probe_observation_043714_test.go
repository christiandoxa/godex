package quota

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type outOfOrderQuotaProbes043714 struct {
	entered      chan int
	releaseFirst chan struct{}
	calls        atomic.Int32
}

func (fixture *outOfOrderQuotaProbes043714) Fetch(_ context.Context, _ string) (quotamodel.Usage, error) {
	index := int(fixture.calls.Add(1))
	fixture.entered <- index
	if index == 1 {
		<-fixture.releaseFirst
	}
	percent := int64(14)
	if index == 1 {
		percent = 100
	}
	return quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &percent}}, nil
}

// Prodex 0.437.14 rejects late successful probe snapshots when a newer
// successful authenticated usage observation was already applied. This
// exercise deliberately returns an earlier exhausted probe last.
func TestProdex043714LateSuccessfulProbeDoesNotOverwriteNewerUsage(t *testing.T) {
	fixture := &outOfOrderQuotaProbes043714{entered: make(chan int, 2), releaseFirst: make(chan struct{})}
	account := accountentity.Account{ID: "account-a", Name: "account-a", Enabled: true}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, fixture)
	now := time.Unix(1500, 0)
	status.now = func() time.Time { return now }
	persisted := &memoryUsageSnapshotStore{}
	status.SetUsageSnapshotStore(persisted)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	firstDone := make(chan error, 1)
	go func() {
		_, err := status.AvailabilityForRoute(ctx, account, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses})
		firstDone <- err
	}()
	select {
	case <-fixture.entered:
	case <-ctx.Done():
		t.Fatal("first probe not started")
	}
	secondDone := make(chan error, 1)
	go func() {
		_, err := status.AvailabilityForRoute(ctx, account, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses})
		secondDone <- err
	}()
	select {
	case <-fixture.entered:
	case <-ctx.Done():
		t.Fatal("newer probe not started")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("newer probe did not finish")
	}
	close(fixture.releaseFirst)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("older probe did not finish")
	}
	status.usageMu.Lock()
	cache := status.usageCache["/managed/account-a"]
	status.usageMu.Unlock()
	if cache.usage.Primary == nil || cache.usage.Primary.UsedPercent == nil || *cache.usage.Primary.UsedPercent != 14 {
		t.Fatalf("late stale successful probe overwrote newer ready quota usage: %#v", cache)
	}
	snapshot, found := persisted.values[account.ID]
	if !found || snapshot.FiveHourStatus == quotamodel.WindowExhausted || snapshot.FiveHourRemainingPercent != 86 {
		t.Fatalf("stale probe overwrote durable quota snapshot: %#v found=%t", snapshot, found)
	}
}
