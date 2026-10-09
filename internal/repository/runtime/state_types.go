package runtime

const (
	runtimeStateCoreMask      = 0x01
	runtimeStateFullMask      = 0x02
	runtimeContinuationsMask  = 0x04
	runtimeProfileScoresMask  = 0x08
	runtimeUsageSnapshotsMask = 0x10
	runtimeBackoffsMask       = 0x20
	runtimeStateMask          = runtimeStateCoreMask | runtimeStateFullMask
	runtimeStateEnvelopeMask  = runtimeStateMask | runtimeProfileScoresMask | runtimeUsageSnapshotsMask | runtimeBackoffsMask
	runtimeSaveMask           = runtimeStateEnvelopeMask | runtimeContinuationsMask
)

// Snapshot is the persisted runtime state grouped by sidecar. Each value is
// JSON and is copied before it crosses the repository boundary.
type Snapshot struct {
	State          []byte
	Continuations  []byte
	ProfileScores  []byte
	UsageSnapshots []byte
	Backoffs       []byte
}
