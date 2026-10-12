package runtime

// NativeRecoveryOutcome is the result of a recovery attempt through an owned
// Codex app-server. It crosses the gateway/usecase boundary without exposing
// protocol details to the usecase.
type NativeRecoveryOutcome uint8

const (
	NativeRecoveryDeferred NativeRecoveryOutcome = iota
	NativeRecoveryStopped
	NativeRecoveryAccepted
	NativeRecoveryAmbiguous
)
