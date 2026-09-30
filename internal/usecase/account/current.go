package account

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

func Current(ctx context.Context, accounts AccountStore) (accountentity.Account, error) {
	return accounts.Current(ctx)
}
