package ping

type Options struct {
	Profile string
	Model   string
	Effort  string
	BaseURL string
	NoProxy bool
	JSON    bool
}

type Target struct {
	Name      string
	CodexHome string
}

type ProcessResult struct {
	Stdout          []byte
	Stderr          []byte
	ExitCode        int
	FirstResponseMS *int64
	LatencyMS       int64
	TimedOut        bool
	Cancelled       bool
	CleanupFailed   bool
}

type Status string

const (
	Pass               Status = "ok"
	AuthFailed         Status = "auth_failed"
	DNSFailed          Status = "dns_failed"
	TLSFailed          Status = "tls_failed"
	Timeout            Status = "timeout"
	RateLimited        Status = "rate_limited"
	QuotaExhausted     Status = "exhausted"
	UpstreamOverloaded Status = "upstream_overloaded"
	ModelUnavailable   Status = "model_unavailable"
	ProtocolFailed     Status = "protocol_failed"
	TurnFailed         Status = "turn_failed"
	ProcessFailed      Status = "process_failed"
	SpawnFailed        Status = "spawn_failed"
	Cancelled          Status = "cancelled"
	UnexpectedResponse Status = "unexpected_response"
)

type Result struct {
	Profile                string `json:"profile"`
	Status                 Status `json:"status"`
	Model                  string `json:"model,omitempty"`
	RequestedModel         string `json:"requested_model,omitempty"`
	Effort                 string `json:"effort"`
	RequestedEffort        string `json:"requested_effort"`
	EffectiveModel         string `json:"effective_model,omitempty"`
	CredentialValidation   string `json:"credential_validation"`
	FirstResponseLatencyMS *int64 `json:"first_response_latency_ms,omitempty"`
	CompletionLatencyMS    *int64 `json:"completion_latency_ms,omitempty"`
	LatencyMS              *int64 `json:"latency_ms,omitempty"`
	Detail                 string `json:"detail"`
}

type Summary struct {
	ProfilesDiscovered int   `json:"profiles_discovered"`
	ProfilesTested     int   `json:"profiles_tested"`
	Healthy            int   `json:"healthy"`
	Exhausted          int   `json:"exhausted"`
	AuthFailures       int   `json:"auth_failures"`
	TemporaryFailures  int   `json:"temporary_failures"`
	OtherFailures      int   `json:"other_failures"`
	DurationMS         int64 `json:"duration_ms"`
	PoolUsable         bool  `json:"pool_usable"`
}

type Report struct {
	Provider        string   `json:"provider"`
	Status          string   `json:"status"`
	Model           string   `json:"model,omitempty"`
	RequestedModel  string   `json:"requested_model,omitempty"`
	Effort          string   `json:"effort"`
	RequestedEffort string   `json:"requested_effort"`
	EffectiveModel  string   `json:"effective_model,omitempty"`
	LatencyMS       int64    `json:"latency_ms"`
	Detail          string   `json:"detail"`
	Profiles        []Result `json:"profiles"`
	Summary         Summary  `json:"summary"`
}
