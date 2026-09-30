package routing

import (
	"encoding/hex"
	"errors"
)

const MaxBindings = 8192
const RetentionSeconds = 30 * 24 * 60 * 60

type Binding struct {
	Kind        string `json:"kind"`
	Key         string `json:"key"`
	AccountID   string `json:"account_id"`
	UpdatedUnix int64  `json:"updated_unix"`
}

func (b Binding) Validate() error {
	switch b.Kind {
	case "previous", "turn", "session", "thread":
	default:
		return errors.New("invalid routing binding kind")
	}
	if len(b.Key) != 64 || len(b.AccountID) != 32 {
		return errors.New("invalid routing binding")
	}
	if _, err := hex.DecodeString(b.Key); err != nil {
		return errors.New("invalid routing key")
	}
	if _, err := hex.DecodeString(b.AccountID); err != nil {
		return errors.New("invalid routing owner")
	}
	return nil
}
