package session

import (
	"context"
	"errors"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

// Recovery of a newly-created child session may release affinity only
// through the configured durable binding store. Missing authorization,
// invalid IDs and unavailable persistence fail closed.
func (catalog *Catalog) ReleaseRecoveryBinding(ctx context.Context, id string) error {
	if catalog == nil || catalog.bindingForget == nil {
		return errors.New("session binding release is not configured")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !sessionentity.ValidID(id) {
		return errors.New("invalid recovery session identity")
	}
	return catalog.bindingForget(ctx, id)
}
