package profile

// ImportLifecycleJournal records a profile bundle import without storing credentials.
type ImportLifecycleJournal struct {
	Version               int                     `json:"version"`
	ID                    string                  `json:"id"`
	Phase                 string                  `json:"phase"`
	Actions               []ImportLifecycleAction `json:"actions"`
	PreviousProfileActive string                  `json:"previous_profile_active,omitempty"`
	PreviousAccountActive string                  `json:"previous_account_active,omitempty"`
	NextProfileActive     string                  `json:"next_profile_active,omitempty"`
	NextAccountActive     string                  `json:"next_account_active,omitempty"`
}

type ImportLifecycleAction struct {
	Name            string                  `json:"name"`
	AccountID       string                  `json:"account_id,omitempty"`
	Create          bool                    `json:"create"`
	Before          *ImportLifecycleProfile `json:"before,omitempty"`
	After           ImportLifecycleProfile  `json:"after"`
	BackupID        string                  `json:"backup_id,omitempty"`
	IdentityCleared bool                    `json:"identity_cleared,omitempty"`
	Files           []ImportLifecycleFile   `json:"files"`
}

type ImportLifecycleProfile struct {
	CodexHome string           `json:"codex_home"`
	Managed   bool             `json:"managed"`
	Email     string           `json:"email,omitempty"`
	Provider  ProviderSnapshot `json:"provider"`
}

type ImportLifecycleFile struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Missing bool   `json:"missing,omitempty"`
}
