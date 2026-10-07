package quota

import (
	"context"
	"testing"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type statusSummaryNoFetch struct{ calls int }

func (source *statusSummaryNoFetch) Fetch(context.Context, string) (quotamodel.Usage, error) {
	source.calls++
	return quotamodel.Usage{}, nil
}

func TestProdex04357StatusSummaryUsesLocalLiveAndProfileNameSnapshots(t *testing.T) {
	now := time.Unix(9_000_000, 0)
	liveUsedFive, liveUsedWeekly := int64(20), int64(30)
	liveFiveReset := now.Add(2 * time.Hour).Unix()
	liveWeeklyReset := now.Add(5 * 24 * time.Hour).Unix()
	liveUsage := quotamodel.Usage{
		Primary:   &quotamodel.Window{UsedPercent: &liveUsedFive, ResetAt: &liveFiveReset},
		Secondary: &quotamodel.Window{UsedPercent: &liveUsedWeekly, ResetAt: &liveWeeklyReset},
	}
	snapshotFiveReset := now.Add(time.Hour).Unix()
	snapshotWeeklyReset := now.Add(6 * 24 * time.Hour).Unix()
	store := &memoryUsageSnapshotStore{values: map[string]quotamodel.UsageSnapshot{
		"standalone": {
			CheckedAt:      now.Add(-2 * time.Minute).Unix(),
			FiveHourStatus: quotamodel.WindowReady, FiveHourRemainingPercent: 60, FiveHourResetAt: snapshotFiveReset,
			WeeklyStatus: quotamodel.WindowReady, WeeklyRemainingPercent: 50, WeeklyResetAt: snapshotWeeklyReset,
		},
	}}
	usage := &statusSummaryNoFetch{}
	status := NewStatus(fakeAccounts{}, usage)
	status.now = func() time.Time { return now }
	status.SetUsageSnapshotStore(store)
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{
		{Name: "main", AccountID: "account-main", CodexHome: "/main", Provider: "openai", Auth: "chatgpt", Enabled: true, Compatible: true},
		{Name: "standalone", CodexHome: "/standalone", Provider: "openai", Auth: "chatgpt", Enabled: true, Compatible: true},
		{Name: "missing", CodexHome: "/missing", Provider: "openai", Auth: "chatgpt", Enabled: true, Compatible: true},
		{Name: "api-key", CodexHome: "/api", Provider: "openai", Auth: "api-key", Enabled: true, Compatible: false},
	}})
	status.storeUsage("/main", liveUsage, now)

	summary, err := status.StatusSummary(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if usage.calls != 0 {
		t.Fatalf("status summary performed network usage fetch %d time(s)", usage.calls)
	}
	if summary.CompatibleProfiles != 3 || summary.UnavailableProfiles != 1 {
		t.Fatalf("profile summary = %#v", summary)
	}
	if summary.FiveHour.Profiles != 2 || summary.FiveHour.TotalRemaining != 140 ||
		summary.FiveHour.EarliestResetAt != snapshotFiveReset {
		t.Fatalf("five-hour summary = %#v", summary.FiveHour)
	}
	if summary.Weekly.Profiles != 2 || summary.Weekly.TotalRemaining != 120 ||
		summary.Weekly.EarliestResetAt != liveWeeklyReset {
		t.Fatalf("weekly summary = %#v", summary.Weekly)
	}
}
