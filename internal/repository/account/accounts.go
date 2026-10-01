package account

import (
	"context"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

type Accounts interface {
	Prepare() error
	TempDir() string
	Root() string
	CodexHome(accountID string) string
	CommitLogin(ctx context.Context, candidate entity.Account, stagedCodexHome string, rename bool) (entity.Account, error)
	List(ctx context.Context) ([]entity.Account, error)
	Current(ctx context.Context) (entity.Account, error)
	Resolve(ctx context.Context, selector string) (entity.Account, error)
	LaunchCandidates(ctx context.Context, selector string) ([]entity.Account, error)
	SelectForLaunch(ctx context.Context, selector string) (entity.Account, error)
	SetActive(ctx context.Context, selector string) (entity.Account, error)
	Remove(ctx context.Context, selector string) (entity.Account, error)
	RemoveProfile(ctx context.Context, selector string, deleteHome bool) (entity.Account, error)
}
