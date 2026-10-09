package runtime

// GoalRecoveryState is a token-free snapshot of the Codex-owned goal row.
type GoalRecoveryState struct {
	SessionID string
	Status    string
	UpdatedAt int64
}
