package runtime

import quotamodel "github.com/christiandoxa/godex/internal/model/quota"

type Event struct {
	TimestampUnixMilli int64             `json:"timestamp_unix_milli"`
	RequestID          string            `json:"request_id,omitempty"`
	Kind               string            `json:"kind"`
	Method             string            `json:"method,omitempty"`
	Path               string            `json:"path,omitempty"`
	AccountID          string            `json:"account_id,omitempty"`
	StatusCode         int               `json:"status_code,omitempty"`
	DurationMillis     int64             `json:"duration_millis,omitempty"`
	Message            string            `json:"message,omitempty"`
	Fields             map[string]string `json:"fields,omitempty"`
}

type TokenUsageCounts struct {
	InputTokens       uint64 `json:"input_tokens"`
	CachedInputTokens uint64 `json:"cached_input_tokens"`
	OutputTokens      uint64 `json:"output_tokens"`
	ReasoningTokens   uint64 `json:"reasoning_tokens"`
}

type TokenUsageSummary struct {
	LogCount   int                         `json:"log_count"`
	EventCount int                         `json:"event_count"`
	Total      TokenUsageCounts            `json:"total"`
	ByProfile  map[string]TokenUsageCounts `json:"by_profile,omitempty"`
}

type RuntimeQuotaObservation struct {
	TimestampUnixMilli int64  `json:"timestamp_unix_milli"`
	Profile            string `json:"profile"`
	FiveHourRemaining  int64  `json:"five_hour_remaining"`
	WeeklyRemaining    int64  `json:"weekly_remaining"`
}

type RunwayEstimate struct {
	BurnPerHour         float64 `json:"burn_per_hour"`
	ObservedProfiles    int     `json:"observed_profiles"`
	ObservedSpanSeconds int64   `json:"observed_span_seconds"`
	ExhaustAt           int64   `json:"exhaust_at"`
}

type RuntimeLoadSummary struct {
	LogCount              int                       `json:"log_count"`
	ActiveInflightUnits   int                       `json:"active_inflight_units"`
	RecentSelectionEvents int                       `json:"recent_selection_events"`
	RecentFirstUnixMilli  int64                     `json:"recent_first_unix_milli,omitempty"`
	RecentLastUnixMilli   int64                     `json:"recent_last_unix_milli,omitempty"`
	Observations          []RuntimeQuotaObservation `json:"observations,omitempty"`
}

type Overview struct {
	GodexHome      string                   `json:"godex_home"`
	CodexVersion   string                   `json:"codex_version"`
	AccountCount   int                      `json:"account_count"`
	ProfileCount   int                      `json:"profile_count"`
	EnabledCount   int                      `json:"enabled_count"`
	ActiveAccount  string                   `json:"active_account"`
	ActiveProfile  string                   `json:"active_profile"`
	RecentEvents   int                      `json:"recent_events"`
	Inflight       int                      `json:"inflight"`
	RuntimeProfile string                   `json:"runtime_profile,omitempty"`
	Quota          quotamodel.StatusSummary `json:"quota"`
	TokenSummary   TokenUsageSummary        `json:"token_summary"`
	TokenHistory   []uint64                 `json:"token_history,omitempty"`
	TokenFirstAt   string                   `json:"token_first_at,omitempty"`
	TokenLastAt    string                   `json:"token_last_at,omitempty"`
	RuntimeLoad      RuntimeLoadSummary       `json:"runtime_load"`
	FiveHourRunway   *RunwayEstimate          `json:"five_hour_runway,omitempty"`
	WeeklyRunway     *RunwayEstimate          `json:"weekly_runway,omitempty"`
	UpdatedUnix      int64                    `json:"updated_unix,omitempty"`
	UpdatedAt      string                   `json:"updated_at,omitempty"`
	LastEvent      *Event                   `json:"last_event,omitempty"`
}
