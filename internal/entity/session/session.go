package session

// Session is Codex-owned rollout metadata, without conversation contents.
type Session struct {
	ID                  string
	ThreadName          string
	Preview             string
	UpdatedAt           string
	CWD                 string
	ModelProvider       string
	LastModel           string
	LastReasoningEffort string
	Source              string
	Path                string
	ParentThreadID      string
	UpdatedUnix         int64
}
