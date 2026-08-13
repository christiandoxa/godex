package account

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

func Use(ctx context.Context, accounts AccountStore, selector string) (accountentity.Account, error) {
	return accounts.SetActive(ctx, selector)
}
