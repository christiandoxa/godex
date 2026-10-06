package quota

type Source uint8

const (
	SourceUnknown Source = iota
	SourceLive
	SourcePersistedSnapshot
)

type WindowStatus string

const (
	WindowReady     WindowStatus = "Ready"
	WindowThin      WindowStatus = "Thin"
	WindowCritical  WindowStatus = "Critical"
	WindowExhausted WindowStatus = "Exhausted"
	WindowUnknown   WindowStatus = "Unknown"
)

type UsageSnapshot struct {
	CheckedAt                int64        `json:"checked_at"`
	PlanType                 *string      `json:"plan_type,omitempty"`
	FiveHourStatus           WindowStatus `json:"five_hour_status"`
	FiveHourRemainingPercent int64        `json:"five_hour_remaining_percent"`
	FiveHourResetAt          int64        `json:"five_hour_reset_at"`
	WeeklyStatus             WindowStatus `json:"weekly_status"`
	WeeklyRemainingPercent   int64        `json:"weekly_remaining_percent"`
	WeeklyResetAt            int64        `json:"weekly_reset_at"`
}
