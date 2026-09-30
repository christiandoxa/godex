package session

type Report struct {
	ID                string `json:"id"`
	ThreadName        string `json:"thread_name,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
	CWD               string `json:"cwd,omitempty"`
	Profile           string `json:"profile"`
	UpstreamAccountID string `json:"-"`
	AccountID         string `json:"-"`
	ModelProvider     string `json:"model_provider,omitempty"`
	Path              string `json:"path"`
	ParentThreadID    string `json:"parent_thread_id,omitempty"`
	UpdatedUnix       int64  `json:"-"`
}

type Query struct {
	CurrentDir string
	Profile    string
	Text       string
	Limit      int
	LimitSet   bool
	ParentOnly bool
}

// Launch carries the resolved transport intent without reparsing CLI arguments.
type Launch struct {
	AccountSelector string
	SessionSelector string
	IDIndex         int
	Arguments       []string
	Local           bool
}
