package quota

type StatusWindowSummary struct {
	Profiles        int   `json:"profiles"`
	TotalRemaining  int64 `json:"total_remaining"`
	EarliestResetAt int64 `json:"earliest_reset_at,omitempty"`
}

type StatusSummary struct {
	CompatibleProfiles  int                 `json:"compatible_profiles"`
	UnavailableProfiles int                 `json:"unavailable_profiles"`
	FiveHour            StatusWindowSummary `json:"five_hour"`
	Weekly              StatusWindowSummary `json:"weekly"`
}
