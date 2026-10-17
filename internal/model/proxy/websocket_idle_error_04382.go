package proxy

import "errors"

// ErrWebSocketCommittedIdleTimeout marks expiry of an already committed
// Responses WebSocket stream. It is NOT a precommit request timeout, not a
// profile quota failure, and never authorizes an inference replay. The
// HTTP WebSocket delivery translates it to an empty close control frame.
var ErrWebSocketCommittedIdleTimeout = errors.New("committed WebSocket stream idle timeout")
