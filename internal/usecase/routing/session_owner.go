package routing

import (
	"context"
	"errors"
	"strings"

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

// ForgetSession removes durable session affinity after a successful native
// session deletion, including the compact-session alias.
func ForgetSession(ctx context.Context, repository interface {
	Remove(context.Context, []string) error
}, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	keys := affinityKeys{thread: id, session: id}.values()
	keys = append(keys, affinityKeys{session: "__compact_session__:" + id}.values()...)
	unique := make(map[string]bool, len(keys))
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		if !unique[key] {
			unique[key] = true
			result = append(result, key)
		}
	}
	return repository.Remove(ctx, result)
}
