package account

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

func Remove(ctx context.Context, accounts AccountStore, selector string) (accountentity.Account, error) {
	return accounts.Remove(ctx, selector)
}
