package account

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type enableStore interface {
	SetEnabled(context.Context, string, bool) (accountentity.Account, error)
}

func Enable(ctx context.Context, store enableStore, selector string, enabled bool) (accountentity.Account, error) {
	return store.SetEnabled(ctx, selector, enabled)
}
