package runtime

import (
	"context"
	"errors"
	"strings"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type activityLog interface {
	Append(context.Context, runtimemodel.Event) error
	Tail(context.Context, int) ([]runtimemodel.Event, error)
}

type activityAccounts interface {
	List(context.Context) ([]accountentity.Account, error)
	Current(context.Context) (accountentity.Account, error)
}

type versionReader interface {
	Version(context.Context) (string, error)
}

type Activity struct {
	home     string
	log      activityLog
	accounts activityAccounts
	codex    versionReader
	now      func() time.Time
}

func NewActivity(home string, log activityLog, accounts activityAccounts, codex versionReader) *Activity {
	return &Activity{home: home, log: log, accounts: accounts, codex: codex, now: time.Now}
}

func (activity *Activity) Record(ctx context.Context, event runtimemodel.Event) error {
	if activity == nil || activity.log == nil {
		return nil
	}
	if strings.TrimSpace(event.Kind) == "" {
		return errors.New("runtime event kind is required")
	}
	if event.TimestampUnixMilli == 0 {
		event.TimestampUnixMilli = activity.now().UnixMilli()
	}
	return activity.log.Append(ctx, event)
}

func (activity *Activity) Events(ctx context.Context, limit int) ([]runtimemodel.Event, error) {
	if activity == nil || activity.log == nil {
		return nil, nil
	}
	return activity.log.Tail(ctx, limit)
}

func (activity *Activity) Overview(ctx context.Context) (runtimemodel.Overview, error) {
	accounts, err := activity.accounts.List(ctx)
	if err != nil {
		return runtimemodel.Overview{}, err
	}
	current, currentErr := activity.accounts.Current(ctx)
	version, versionErr := activity.codex.Version(ctx)
	if versionErr != nil {
		version = "unavailable"
	}
	events, err := activity.Events(ctx, 512)
	if err != nil {
		return runtimemodel.Overview{}, err
	}
	overview := runtimemodel.Overview{
		GodexHome:    activity.home,
		CodexVersion: version,
		AccountCount: len(accounts),
		RecentEvents: len(events),
		Inflight:     inflightCount(events),
	}
	if currentErr == nil {
		overview.ActiveAccount = current.Name
	}
	for _, account := range accounts {
		if account.Enabled {
			overview.EnabledCount++
		}
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		overview.LastEvent = &last
	}
	return overview, nil
}

func inflightCount(events []runtimemodel.Event) int {
	active := make(map[string]bool)
	for _, event := range events {
		if event.RequestID == "" {
			continue
		}
		switch event.Kind {
		case "request_started":
			active[event.RequestID] = true
		case "request_completed", "request_failed":
			delete(active, event.RequestID)
		}
	}
	return len(active)
}
