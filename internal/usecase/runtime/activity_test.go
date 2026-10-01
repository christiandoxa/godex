package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
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
