package account

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

func List(ctx context.Context, accounts AccountStore) ([]accountentity.Account, error) {
	return accounts.List(ctx)
}
