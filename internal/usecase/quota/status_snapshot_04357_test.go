package quota

import (
	"testing"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04357StandaloneProfileLiveQuotaPersistsProfileNameSnapshot(t *testing.T) {
	now := time.Unix(9_100_000, 0)
	usedFive, usedWeekly := int64(25), int64(40)
	fiveReset := now.Add(time.Hour).Unix()
	weeklyReset := now.Add(6 * 24 * time.Hour).Unix()
	source := &fakeOverrideUsage{usage: quotamodel.Usage{
		Primary:   &quotamodel.Window{UsedPercent: &usedFive, ResetAt: &fiveReset},
		Secondary: &quotamodel.Window{UsedPercent: &usedWeekly, ResetAt: &weeklyReset},
	}}
	store := &memoryUsageSnapshotStore{}
	status := NewStatus(fakeAccounts{}, source)
	status.now = func() time.Time { return now }
	status.SetUsageSnapshotStore(store)
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "standalone", CodexHome: "/profiles/standalone",
		Provider: "openai", Auth: "chatgpt", Active: true, Enabled: true, Compatible: true,
	}}})

	reports, err := status.Run(t.Context(), Options{})
	if err != nil || len(reports) != 1 || reports[0].Err != nil {
		t.Fatalf("quota reports = %#v err=%v", reports, err)
	}
	snapshot, ok := store.values["standalone"]
	if !ok {
		t.Fatalf("profile-name snapshot was not persisted: %#v", store.values)
	}
	if snapshot.FiveHourRemainingPercent != 75 || snapshot.WeeklyRemainingPercent != 60 {
		t.Fatalf("standalone snapshot = %#v", snapshot)
	}
}
