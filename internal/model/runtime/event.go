package runtime

type Event struct {
	TimestampUnixMilli int64  `json:"timestamp_unix_milli"`
	RequestID          string `json:"request_id,omitempty"`
	Kind               string `json:"kind"`
	Method             string `json:"method,omitempty"`
	Path               string `json:"path,omitempty"`
	AccountID          string `json:"account_id,omitempty"`
	StatusCode         int    `json:"status_code,omitempty"`
	DurationMillis     int64  `json:"duration_millis,omitempty"`
	Message            string `json:"message,omitempty"`
}

type Overview struct {
	GodexHome     string `json:"godex_home"`
	CodexVersion  string `json:"codex_version"`
	AccountCount  int    `json:"account_count"`
	ProfileCount  int    `json:"profile_count"`
	EnabledCount  int    `json:"enabled_count"`
	ActiveAccount string `json:"active_account"`
	ActiveProfile string `json:"active_profile"`
	RecentEvents  int    `json:"recent_events"`
	Inflight      int    `json:"inflight"`
	LastEvent     *Event `json:"last_event,omitempty"`
}
