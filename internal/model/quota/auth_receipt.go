package quota

import "time"

// AuthenticatedReceipt identifies credentials actually accepted by an HTTP
// quota request. It never stores tokens or user-facing account data, and a
// cached usage snapshot must not fabricate it.
type AuthenticatedReceipt struct {
	CredentialDigest [32]byte
	StartedAt        time.Time
	Confirmed        bool
}
