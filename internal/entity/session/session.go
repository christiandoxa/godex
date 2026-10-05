package session

// Session is Codex-owned rollout metadata, without conversation contents.
type Session struct {
	ID             string
	ThreadName     string
	UpdatedAt      string
	CWD            string
	ModelProvider  string
	Source         string
	Path           string
	ParentThreadID string
	UpdatedUnix    int64
}
