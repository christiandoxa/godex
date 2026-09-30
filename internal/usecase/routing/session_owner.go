package routing

import (
	"context"
	"errors"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

// SessionOwner resolves the durable upstream owner, independently of rollout location.
func SessionOwner(ctx context.Context, repository interface {
	Load(context.Context) ([]routingentity.Binding, error)
}, id string) (string, error) {
	bindings, err := repository.Load(ctx)
	if err != nil {
		return "", err
	}
	keys := affinityKeys{thread: id, session: id}.values()
	owner := ""
	for _, binding := range bindings {
		for _, key := range keys {
			if binding.Key != key {
				continue
			}
			if owner != "" && owner != binding.AccountID {
				return "", errors.New("session has conflicting upstream ownership")
			}
			owner = binding.AccountID
		}
	}
	return owner, nil
}
