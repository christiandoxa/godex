package routing

import (
	"encoding/hex"
	"errors"
)

// ContinuationStatus is the durable, hashed terminal state for a continuation.
// It deliberately contains no account identifier or opaque continuation value.
type ContinuationStatus struct {
	Kind        string `json:"kind"`
	Key         string `json:"key"`
	State       string `json:"state"`
	UpdatedUnix int64  `json:"updated_unix"`
}

func (status ContinuationStatus) Validate() error {
	switch status.Kind {
	case "response", "turn_state", "session_id":
	default:
		return errors.New("invalid continuation status kind")
	}
	if status.State != "dead" || status.UpdatedUnix < 0 || len(status.Key) != 64 {
		return errors.New("invalid continuation status")
	}
	if _, err := hex.DecodeString(status.Key); err != nil {
		return errors.New("invalid continuation status key")
	}
	return nil
}
