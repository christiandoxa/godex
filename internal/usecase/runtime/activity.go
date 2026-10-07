package runtime

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

const statusOverviewEventLimit = 65_536

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

type activityProfiles interface {
	Summary(context.Context) (profilemodel.Summary, error)
}

type activityQuota interface {
	StatusSummary(context.Context) (quotamodel.StatusSummary, error)
}

type activityRuntimeProfiles interface {
	SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error)
}

type Activity struct {
	home     string
	log      activityLog
	accounts activityAccounts
	codex    versionReader
	profiles activityProfiles
	quota    activityQuota
	now      func() time.Time
}

func NewActivity(home string, log activityLog, accounts activityAccounts, codex versionReader) *Activity {
	return &Activity{home: home, log: log, accounts: accounts, codex: codex, now: time.Now}
}

func (activity *Activity) SetProfiles(profiles activityProfiles) {
	activity.profiles = profiles
}

func (activity *Activity) SetQuotaStatus(quota activityQuota) {
	activity.quota = quota
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
	events, err := activity.Events(ctx, statusOverviewEventLimit)
	if err != nil {
		return runtimemodel.Overview{}, err
	}
	now := activity.now()
	overview := runtimemodel.Overview{
		GodexHome:    activity.home,
		CodexVersion: version,
		AccountCount: len(accounts),
		ProfileCount: len(accounts),
		RecentEvents: len(events),
		Inflight:     inflightCount(events),
		TokenSummary: tokenUsageSummary(events),
		TokenHistory: tokenUsageHistory(events, 64),
		RuntimeLoad:  runtimeLoadSummary(events, now),
		UpdatedAt:    now.In(time.Local).Format("2006-01-02 15:04:05"),
		UpdatedUnix:  now.Unix(),
	}
	overview.TokenFirstAt, overview.TokenLastAt = tokenUsageRange(events)
	if currentErr == nil {
		overview.ActiveAccount = current.Name
		overview.ActiveProfile = current.Name
	}
	if activity.profiles != nil {
		summary, err := activity.profiles.Summary(ctx)
		if err != nil {
			return runtimemodel.Overview{}, err
		}
		overview.ProfileCount = summary.Count
		overview.ActiveProfile = summary.Active
	}
	if activity.quota != nil {
		quota, err := activity.quota.StatusSummary(ctx)
		if err != nil {
			return runtimemodel.Overview{}, err
		}
		overview.Quota = quota
	}
	overview.FiveHourRunway = estimateRuntimeRunway(
		overview.RuntimeLoad.Observations, false, overview.Quota.FiveHour.TotalRemaining, overview.UpdatedUnix,
	)
	overview.WeeklyRunway = estimateRuntimeRunway(
		overview.RuntimeLoad.Observations, true, overview.Quota.Weekly.TotalRemaining, overview.UpdatedUnix,
	)
	for _, account := range accounts {
		if account.Enabled {
			overview.EnabledCount++
		}
	}
	overview.RuntimeProfile = overview.ActiveProfile
	if source, ok := activity.profiles.(activityRuntimeProfiles); ok {
		if homes, err := source.SessionProfiles(ctx); err == nil {
			if profile := runtimeProfileFromEvents(events, homes); profile != "" {
				overview.RuntimeProfile = profile
			}
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

func tokenUsageSummary(events []runtimemodel.Event) runtimemodel.TokenUsageSummary {
	summary := runtimemodel.TokenUsageSummary{ByProfile: make(map[string]runtimemodel.TokenUsageCounts)}
	if len(events) > 0 {
		summary.LogCount = 1
	}
	for _, event := range events {
		if event.Kind != "token_usage" {
			continue
		}
		counts := tokenCountsFromEvent(event)
		summary.EventCount++
		addTokenCounts(&summary.Total, counts)
		profile := strings.TrimSpace(event.Fields["profile"])
		if profile == "" {
			profile = strings.TrimSpace(event.AccountID)
		}
		if profile != "" {
			current := summary.ByProfile[profile]
			addTokenCounts(&current, counts)
			summary.ByProfile[profile] = current
		}
	}
	return summary
}

func tokenUsageHistory(events []runtimemodel.Event, limit int) []uint64 {
	if limit <= 0 {
		return nil
	}
	values := make([]uint64, 0, min(limit, len(events)))
	for _, event := range events {
		if event.Kind != "token_usage" {
			continue
		}
		counts := tokenCountsFromEvent(event)
		value := counts.InputTokens + counts.OutputTokens
		if len(values) == limit {
			copy(values, values[1:])
			values[len(values)-1] = value
		} else {
			values = append(values, value)
		}
	}
	return values
}

func tokenUsageRange(events []runtimemodel.Event) (string, string) {
	var first, last int64
	for _, event := range events {
		if event.Kind != "token_usage" {
			continue
		}
		if first == 0 {
			first = event.TimestampUnixMilli
		}
		last = event.TimestampUnixMilli
	}
	format := func(value int64) string {
		if value == 0 {
			return ""
		}
		return time.UnixMilli(value).In(time.Local).Format("2006-01-02 15:04:05")
	}
	return format(first), format(last)
}

func tokenCountsFromEvent(event runtimemodel.Event) runtimemodel.TokenUsageCounts {
	read := func(key string) uint64 {
		value, _ := strconv.ParseUint(strings.TrimSpace(event.Fields[key]), 10, 64)
		return value
	}
	return runtimemodel.TokenUsageCounts{
		InputTokens: read("input_tokens"), CachedInputTokens: read("cached_input_tokens"),
		OutputTokens: read("output_tokens"), ReasoningTokens: read("reasoning_tokens"),
	}
}

func addTokenCounts(target *runtimemodel.TokenUsageCounts, value runtimemodel.TokenUsageCounts) {
	target.InputTokens += value.InputTokens
	target.CachedInputTokens += value.CachedInputTokens
	target.OutputTokens += value.OutputTokens
	target.ReasoningTokens += value.ReasoningTokens
}

func runtimeLoadSummary(events []runtimemodel.Event, now time.Time) runtimemodel.RuntimeLoadSummary {
	summary := runtimemodel.RuntimeLoadSummary{}
	if len(events) > 0 {
		summary.LogCount = 1
	}
	latestInflight := make(map[string]int)
	recentCutoff := now.Add(-30 * time.Minute).UnixMilli()
	lookbackCutoff := now.Add(-3 * time.Hour).UnixMilli()
	for _, event := range events {
		if event.Kind == "profile_inflight" {
			profile := strings.TrimSpace(event.Fields["profile"])
			count, err := strconv.Atoi(strings.TrimSpace(event.Fields["count"]))
			if profile != "" && err == nil {
				latestInflight[profile] = max(count, 0)
			}
		}
		observation, ok := runtimeQuotaObservation(event)
		if !ok {
			continue
		}
		if observation.TimestampUnixMilli >= lookbackCutoff {
			summary.Observations = append(summary.Observations, observation)
		}
		if observation.TimestampUnixMilli >= recentCutoff {
			summary.RecentSelectionEvents++
			if summary.RecentFirstUnixMilli == 0 || observation.TimestampUnixMilli < summary.RecentFirstUnixMilli {
				summary.RecentFirstUnixMilli = observation.TimestampUnixMilli
			}
			if observation.TimestampUnixMilli > summary.RecentLastUnixMilli {
				summary.RecentLastUnixMilli = observation.TimestampUnixMilli
			}
		}
	}
	for _, count := range latestInflight {
		summary.ActiveInflightUnits += count
	}
	sort.Slice(summary.Observations, func(i, j int) bool {
		left, right := summary.Observations[i], summary.Observations[j]
		if left.TimestampUnixMilli != right.TimestampUnixMilli {
			return left.TimestampUnixMilli < right.TimestampUnixMilli
		}
		return left.Profile < right.Profile
	})
	return summary
}

func runtimeQuotaObservation(event runtimemodel.Event) (runtimemodel.RuntimeQuotaObservation, bool) {
	if event.Kind != "selection_pick" && event.Kind != "selection_keep_current" {
		return runtimemodel.RuntimeQuotaObservation{}, false
	}
	profile := strings.TrimSpace(event.Fields["profile"])
	if profile == "" || profile == "none" {
		return runtimemodel.RuntimeQuotaObservation{}, false
	}
	five, err := strconv.ParseInt(strings.TrimSpace(event.Fields["five_hour_remaining"]), 10, 64)
	if err != nil {
		return runtimemodel.RuntimeQuotaObservation{}, false
	}
	weekly, err := strconv.ParseInt(strings.TrimSpace(event.Fields["weekly_remaining"]), 10, 64)
	if err != nil {
		return runtimemodel.RuntimeQuotaObservation{}, false
	}
	return runtimemodel.RuntimeQuotaObservation{
		TimestampUnixMilli: event.TimestampUnixMilli,
		Profile:            profile,
		FiveHourRemaining:  five,
		WeeklyRemaining:    weekly,
	}, true
}

func estimateRuntimeRunway(
	observations []runtimemodel.RuntimeQuotaObservation,
	weekly bool,
	currentRemaining int64,
	now int64,
) *runtimemodel.RunwayEstimate {
	if currentRemaining <= 0 {
		return &runtimemodel.RunwayEstimate{ExhaustAt: now}
	}
	byProfile := make(map[string][]runtimemodel.RuntimeQuotaObservation)
	for _, observation := range observations {
		byProfile[observation.Profile] = append(byProfile[observation.Profile], observation)
	}
	burnPerHour := 0.0
	observedProfiles := 0
	earliest := int64(math.MaxInt64)
	latest := int64(math.MinInt64)
	for _, values := range byProfile {
		sort.Slice(values, func(i, j int) bool {
			return values[i].TimestampUnixMilli < values[j].TimestampUnixMilli
		})
		burn, start, end, ok := runtimeProfileBurnRate(values, weekly, 5*time.Minute)
		if !ok {
			continue
		}
		burnPerHour += burn
		observedProfiles++
		earliest = min(earliest, start)
		latest = max(latest, end)
	}
	if burnPerHour <= 0 || observedProfiles == 0 || earliest == math.MaxInt64 || latest == math.MinInt64 {
		return nil
	}
	secondsUntilExhaustion := int64(math.Ceil(float64(currentRemaining) / burnPerHour * 3600))
	return &runtimemodel.RunwayEstimate{
		BurnPerHour:         burnPerHour,
		ObservedProfiles:    observedProfiles,
		ObservedSpanSeconds: max(int64(0), latest-earliest),
		ExhaustAt:           now + max(secondsUntilExhaustion, int64(0)),
	}
}

func runtimeProfileBurnRate(
	observations []runtimemodel.RuntimeQuotaObservation,
	weekly bool,
	minimumSpan time.Duration,
) (float64, int64, int64, bool) {
	if len(observations) < 2 {
		return 0, 0, 0, false
	}
	latest := observations[len(observations)-1]
	earliest := latest
	currentRemaining := runwayObservationRemaining(latest, weekly)
	for index := len(observations) - 2; index >= 0; index-- {
		observation := observations[index]
		remaining := runwayObservationRemaining(observation, weekly)
		if remaining < currentRemaining {
			break
		}
		earliest = observation
		currentRemaining = remaining
	}
	earliestRemaining := runwayObservationRemaining(earliest, weekly)
	latestRemaining := runwayObservationRemaining(latest, weekly)
	burned := earliestRemaining - latestRemaining
	start := earliest.TimestampUnixMilli / 1000
	end := latest.TimestampUnixMilli / 1000
	spanSeconds := end - start
	if burned <= 0 || spanSeconds < int64(minimumSpan/time.Second) {
		return 0, 0, 0, false
	}
	return float64(burned) * 3600 / float64(spanSeconds), start, end, true
}

func runwayObservationRemaining(observation runtimemodel.RuntimeQuotaObservation, weekly bool) int64 {
	if weekly {
		return observation.WeeklyRemaining
	}
	return observation.FiveHourRemaining
}

func runtimeSelectionEvent(kind string) bool {
	switch kind {
	case "route_decision", "selection_plan", "selection_pick", "profile_commit", "runtime_recovery":
		return true
	default:
		return strings.HasPrefix(kind, "selection_")
	}
}

func runtimeProfileFromEvents(events []runtimemodel.Event, homes []sessionmodel.ProfileHome) string {
	if len(homes) == 0 {
		return ""
	}
	ids := make(map[string]string)
	for _, home := range homes {
		if home.Name != "" {
			ids[home.Name] = home.Name
		}
		if home.AccountID != "" {
			ids[home.AccountID] = home.Name
		}
		for _, id := range home.RoutingIDs {
			if id != "" {
				ids[id] = home.Name
			}
		}
	}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		candidates := []string{event.Fields["profile"], event.AccountID}
		for _, candidate := range candidates {
			if name := ids[strings.TrimSpace(candidate)]; name != "" {
				return name
			}
		}
	}
	return ""
}

func sortedTokenProfiles(summary runtimemodel.TokenUsageSummary) []string {
	keys := make([]string, 0, len(summary.ByProfile))
	for key := range summary.ByProfile {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
