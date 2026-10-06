package quota

import (
	"context"
	"errors"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type memoryUsageSnapshotStore struct {
	values map[string]quotamodel.UsageSnapshot
}

func (store *memoryUsageSnapshotStore) Load(_ context.Context, accountID string) (quotamodel.UsageSnapshot, bool, error) {
	value, ok := store.values[accountID]
	return value, ok, nil
}
func (store *memoryUsageSnapshotStore) Save(_ context.Context, accountID string, value quotamodel.UsageSnapshot) error {
	if store.values == nil {
		store.values = make(map[string]quotamodel.UsageSnapshot)
	}
	store.values[accountID] = value
	return nil
}

type failingUsage struct{}

func (failingUsage) Fetch(context.Context, string) (quotamodel.Usage, error) {
	return quotamodel.Usage{}, errors.New("synthetic live quota failure")
}

func TestProdex04355QuotaFallsBackToPersistedSnapshotAfterFreshWindow(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	usedFive, usedWeekly := int64(20), int64(30)
	fiveReset, weeklyReset := now.Add(time.Hour).Unix(), now.Add(7*24*time.Hour).Unix()
	usage := quotamodel.Usage{
		PlanType: "plus", RateLimitPresent: true,
		Primary:   &quotamodel.Window{UsedPercent: &usedFive, ResetAt: &fiveReset},
		Secondary: &quotamodel.Window{UsedPercent: &usedWeekly, ResetAt: &weeklyReset},
	}
	account := accountentity.Account{ID: "account-a", Name: "main", Enabled: true}
	accounts := fakeAccounts{accounts: []accountentity.Account{account}, current: account}
	store := &memoryUsageSnapshotStore{}
	live := NewStatus(accounts, fakeUsage{byHome: map[string]quotamodel.Usage{"/managed/account-a": usage}})
	live.now = func() time.Time { return now }
	live.SetUsageSnapshotStore(store)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	availability, err := live.AvailabilityForRoute(t.Context(), account, selection)
	if err != nil || availability.Source != quotamodel.SourceLive {
		t.Fatalf("live availability = %#v err=%v", availability, err)
	}

	restartedNow := now.Add(6 * time.Minute)
	restarted := NewStatus(accounts, failingUsage{})
	restarted.now = func() time.Time { return restartedNow }
	restarted.SetUsageSnapshotStore(store)
	availability, err = restarted.AvailabilityForRoute(t.Context(), account, selection)
	if err != nil || availability.Source != quotamodel.SourcePersistedSnapshot || !availability.Ready {
		t.Fatalf("snapshot availability = %#v err=%v", availability, err)
	}
	if availability.Pressure.FiveHourRemaining != 80 || availability.Pressure.WeeklyRemaining != 70 {
		t.Fatalf("snapshot pressure = %#v", availability.Pressure)
	}
}

func TestProdex04355UsageSnapshotUsabilityMatchesTaggedHoldPolicy(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	ready := quotamodel.UsageSnapshot{
		CheckedAt:      now.Add(-29 * time.Minute).Unix(),
		FiveHourStatus: quotamodel.WindowReady, FiveHourRemainingPercent: 80, FiveHourResetAt: now.Add(time.Hour).Unix(),
		WeeklyStatus: quotamodel.WindowReady, WeeklyRemainingPercent: 80, WeeklyResetAt: now.Add(7 * 24 * time.Hour).Unix(),
	}
	if !usageSnapshotUsable(ready, now) {
		t.Fatal("29-minute non-exhausted snapshot should remain usable")
	}
	ready.CheckedAt = now.Add(-31 * time.Minute).Unix()
	if usageSnapshotUsable(ready, now) {
		t.Fatal("31-minute non-exhausted snapshot remained usable")
	}
	exhausted := ready
	exhausted.FiveHourStatus = quotamodel.WindowExhausted
	exhausted.FiveHourRemainingPercent = 0
	exhausted.FiveHourResetAt = now.Add(time.Hour).Unix()
	if !usageSnapshotUsable(exhausted, now) {
		t.Fatal("active exhausted hold should remain authoritative beyond stale grace")
	}
	exhausted.FiveHourResetAt = now.Unix()
	if usageSnapshotUsable(exhausted, now) {
		t.Fatal("expired exhausted hold remained usable")
	}
}

func TestProdex04355UsageSnapshotUsesTaggedWindowStatusAndNeutralMissingWindow(t *testing.T) {
	now := time.Unix(3_000_000, 0)
	used := int64(90) // 10% remaining => tagged generic Thin.
	reset := now.Add(time.Hour).Unix()
	snapshot := usageSnapshotFromUsage(quotamodel.Usage{
		Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset},
	}, now)
	if snapshot.FiveHourStatus != quotamodel.WindowThin || snapshot.FiveHourRemainingPercent != 10 {
		t.Fatalf("five-hour snapshot = %#v", snapshot)
	}
	if snapshot.WeeklyStatus != quotamodel.WindowReady || snapshot.WeeklyRemainingPercent != 100 || snapshot.WeeklyResetAt != int64(^uint64(0)>>1) {
		t.Fatalf("neutral weekly snapshot = %#v", snapshot)
	}
}

func TestProdex04355PersistedSnapshotDoesNotStandInForMissingLunaReserve(t *testing.T) {
	now := time.Unix(4_000_000, 0)
	used := int64(100)
	reset := now.Add(time.Hour).Unix()
	usage := quotamodel.Usage{
		Primary:   &quotamodel.Window{UsedPercent: &used, ResetAt: &reset},
		Secondary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset},
	}
	if persistedSnapshotSupportsSelection(usage, quotamodel.Selection{RequestedModel: "gpt-5.6-luna"}, now) {
		t.Fatal("regular exhausted snapshot incorrectly stood in for missing Luna reserve")
	}
	if !persistedSnapshotSupportsSelection(usage, quotamodel.Selection{RequestedModel: "gpt-5.6-sol"}, now) {
		t.Fatal("regular snapshot was rejected for standard model")
	}
}
