package openai

import (
	"errors"
	"net/http"
	"time"
)

// Prodex 0.438.2 response_tracking/commit.rs restores the *configured*
// upstream stream idle timeout only after a model output commits. This is
// distinct from both the request-local precommit watchdog and the fixed
// eight-second per-frame precommit progress timer. Never alter those other
// two timer owners when a committed idle override is installed.
func NewTransportWithCommittedWebSocketIdle04382(
	upstream string, client *http.Client, auth authReader, configured time.Duration,
) (*Transport, error) {
	if configured < 0 {
		return nil, errors.New("committed WebSocket stream idle timeout must be nonnegative")
	}
	transport, err := NewTransport(upstream, client, auth)
	if err != nil {
		return nil, err
	}
	transport.websocketCommittedIdle04382 = configured
	return transport, nil
}

func (transport *Transport) CommittedWebSocketIdleTimeout04382() time.Duration {
	if transport.websocketCommittedIdle04382 > 0 {
		return transport.websocketCommittedIdle04382
	}
	return websocketCommittedStreamIdleTimeout
}
