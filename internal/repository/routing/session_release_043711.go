package routing

import (
	"encoding/hex"
	"errors"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

// A session release fence is intentionally independent of a transient
// verified/dead status. It is persisted in the same atomic snapshot as the
// bindings, and is applied before any older worker snapshot is merged.
// Previous-response and turn-state identities are never affected.
func validateSessionReleaseEpochs(epochs map[string]int64) error {
	for key, epoch := range epochs {
		if len(key) != 64 || epoch < 0 {
			return errors.New("invalid routing session release fence")
		}
		if _, err := hex.DecodeString(key); err != nil {
			return errors.New("invalid routing session release key")
		}
	}
	return nil
}

func filterReleasedSessionBindings(bindings []routingentity.Binding, epochs map[string]int64) []routingentity.Binding {
	if len(epochs) == 0 {
		return bindings
	}
	active := make([]routingentity.Binding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Kind == "session" {
			if barrier, ok := epochs[binding.Key]; ok && binding.UpdatedUnix <= barrier {
				continue
			}
		}
		active = append(active, binding)
	}
	return active
}
