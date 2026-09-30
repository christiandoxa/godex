package account

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type AccountStore interface {
	List(context.Context) ([]accountentity.Account, error)
	Current(context.Context) (accountentity.Account, error)
	SetActive(context.Context, string) (accountentity.Account, error)
	Remove(context.Context, string) (accountentity.Account, error)
}
