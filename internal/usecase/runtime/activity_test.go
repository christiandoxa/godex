package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

type activityMemoryLog struct {
	events []runtimemodel.Event
}

func (log *activityMemoryLog) Append(_ context.Context, event runtimemodel.Event) error {
	log.events = append(log.events, event)
	return nil
}

func (log *activityMemoryLog) Tail(_ context.Context, limit int) ([]runtimemodel.Event, error) {
	start := 0
	if len(log.events) > limit {
		start = len(log.events) - limit
	}
	return append([]runtimemodel.Event(nil), log.events[start:]...), nil
}

type activityTestAccounts struct {
	accounts []accountentity.Account
	current  accountentity.Account
}

func (accounts activityTestAccounts) List(context.Context) ([]accountentity.Account, error) {
	return append([]accountentity.Account(nil), accounts.accounts...), nil
}

func (accounts activityTestAccounts) Current(context.Context) (accountentity.Account, error) {
	if accounts.current.ID == "" {
		return accountentity.Account{}, errors.New("no current account")
	}
	return accounts.current, nil
}

type activityVersion string

func (version activityVersion) Version(context.Context) (string, error) {
	return string(version), nil
}

func TestActivityRecordsAndSummarizesRuntime(t *testing.T) {
	now := time.Unix(100, 0)
	log := &activityMemoryLog{}
	current := accountentity.Account{ID: "one", Name: "work", Enabled: true}
	activity := NewActivity("/managed", log, activityTestAccounts{
		accounts: []accountentity.Account{current, {ID: "two", Name: "off", Enabled: false}},
		current:  current,
	}, activityVersion("codex-cli 0.159.2"))
	activity.now = func() time.Time { return now }
	for _, event := range []runtimemodel.Event{
		{Kind: "request_started", RequestID: "one"},
		{Kind: "request_started", RequestID: "two"},
		{Kind: "request_completed", RequestID: "one", StatusCode: 200},
	} {
		if err := activity.Record(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	overview, err := activity.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if overview.AccountCount != 2 || overview.EnabledCount != 1 || overview.ActiveAccount != "work" {
		t.Fatalf("overview accounts = %#v", overview)
	}
	if overview.Inflight != 1 || overview.RecentEvents != 3 || overview.LastEvent == nil || overview.LastEvent.StatusCode != 200 {
		t.Fatalf("overview activity = %#v", overview)
	}
	if log.events[0].TimestampUnixMilli != now.UnixMilli() {
		t.Fatalf("timestamp = %d", log.events[0].TimestampUnixMilli)
	}
}

func TestActivityRejectsEmptyEventKind(t *testing.T) {
	activity := NewActivity("/managed", &activityMemoryLog{}, activityTestAccounts{}, activityVersion("codex"))
	if err := activity.Record(context.Background(), runtimemodel.Event{}); err == nil {
		t.Fatal("empty runtime event unexpectedly accepted")
	}
}

type activityTestProfiles struct {
	summary profilemodel.Summary
	homes   []sessionmodel.ProfileHome
}

func (profiles activityTestProfiles) Summary(context.Context) (profilemodel.Summary, error) {
	return profiles.summary, nil
}

func (profiles activityTestProfiles) SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error) {
	return append([]sessionmodel.ProfileHome(nil), profiles.homes...), nil
}

type activityTestQuota struct{ summary quotamodel.StatusSummary }

func (quota activityTestQuota) StatusSummary(context.Context) (quotamodel.StatusSummary, error) {
	return quota.summary, nil
}

func TestActivityOverviewUsesMergedProfileSummary(t *testing.T) {
	current := accountentity.Account{ID: "one", Name: "account", Enabled: true}
	activity := NewActivity("/managed", &activityMemoryLog{}, activityTestAccounts{
		accounts: []accountentity.Account{current}, current: current,
	}, activityVersion("codex-cli 0.159.2"))
	activity.SetProfiles(activityTestProfiles{summary: profilemodel.Summary{Count: 3, Active: "standalone"}})
	overview, err := activity.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if overview.AccountCount != 1 || overview.ProfileCount != 3 || overview.ActiveProfile != "standalone" || overview.ActiveAccount != "account" {
		t.Fatalf("overview = %#v", overview)
	}
}

func TestProdex04357ActivityOverviewAggregatesQuotaTokensLoadAndRuntimeProfile(t *testing.T) {
	now := time.Date(2026, 6, 20, 1, 6, 0, 0, time.UTC)
	log := &activityMemoryLog{events: []runtimemodel.Event{
		{TimestampUnixMilli: now.Add(-5 * time.Minute).UnixMilli(), Kind: "request_started", RequestID: "req-1", AccountID: "route-main"},
		{TimestampUnixMilli: now.Add(-4 * time.Minute).UnixMilli(), Kind: "selection_pick", AccountID: "route-main", Fields: map[string]string{
			"profile": "route-main", "five_hour_remaining": "70", "weekly_remaining": "60",
		}},
		{TimestampUnixMilli: now.Add(-3500 * time.Millisecond).UnixMilli(), Kind: "profile_inflight", AccountID: "route-main", Fields: map[string]string{
			"profile": "route-main", "count": "1",
		}},
		{TimestampUnixMilli: now.Add(-3 * time.Minute).UnixMilli(), Kind: "token_usage", AccountID: "route-main", Fields: map[string]string{
			"profile": "route-main", "input_tokens": "100", "cached_input_tokens": "25", "output_tokens": "40", "reasoning_tokens": "8",
		}},
		{TimestampUnixMilli: now.Add(-time.Minute).UnixMilli(), Kind: "token_usage", AccountID: "route-backup", Fields: map[string]string{
			"profile": "route-backup", "input_tokens": "10", "cached_input_tokens": "0", "output_tokens": "4", "reasoning_tokens": "1",
		}},
	}}
	current := accountentity.Account{ID: "one", Name: "configured-main", Enabled: true}
	activity := NewActivity("/managed", log, activityTestAccounts{
		accounts: []accountentity.Account{current}, current: current,
	}, activityVersion("codex-cli 0.160.1"))
	activity.now = func() time.Time { return now }
	activity.SetProfiles(activityTestProfiles{
		summary: profilemodel.Summary{Count: 2, Active: "configured-main"},
		homes: []sessionmodel.ProfileHome{
			{Name: "configured-main", AccountID: "one", RoutingIDs: []string{"route-main"}},
			{Name: "runtime-backup", RoutingIDs: []string{"route-backup"}},
		},
	})
	activity.SetQuotaStatus(activityTestQuota{summary: quotamodel.StatusSummary{
		CompatibleProfiles: 2, UnavailableProfiles: 1,
		FiveHour: quotamodel.StatusWindowSummary{Profiles: 2, TotalRemaining: 140},
	}})
	overview, err := activity.Overview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if overview.RuntimeProfile != "runtime-backup" || overview.ActiveProfile != "configured-main" ||
		overview.Quota.CompatibleProfiles != 2 || overview.Quota.UnavailableProfiles != 1 {
		t.Fatalf("profile/quota overview = %#v", overview)
	}
	if overview.TokenSummary.EventCount != 2 || overview.TokenSummary.LogCount != 1 ||
		overview.TokenSummary.Total.InputTokens != 110 ||
		overview.TokenSummary.Total.CachedInputTokens != 25 ||
		overview.TokenSummary.Total.OutputTokens != 44 ||
		overview.TokenSummary.Total.ReasoningTokens != 9 {
		t.Fatalf("token summary = %#v", overview.TokenSummary)
	}
	if len(overview.TokenHistory) != 2 || overview.TokenHistory[0] != 140 || overview.TokenHistory[1] != 14 {
		t.Fatalf("token history = %#v", overview.TokenHistory)
	}
	if overview.RuntimeLoad.RecentSelectionEvents != 1 || overview.RuntimeLoad.ActiveInflightUnits != 1 ||
		overview.RuntimeLoad.LogCount != 1 {
		t.Fatalf("runtime load = %#v", overview.RuntimeLoad)
	}
	if overview.TokenFirstAt == "" || overview.TokenLastAt == "" || overview.UpdatedAt == "" {
		t.Fatalf("token/update timestamps = first:%q last:%q updated:%q", overview.TokenFirstAt, overview.TokenLastAt, overview.UpdatedAt)
	}
}
