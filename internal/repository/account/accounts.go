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
	ActiveID(ctx context.Context) (string, error)
	Resolve(ctx context.Context, selector string) (entity.Account, error)
	LaunchCandidates(ctx context.Context, selector string) ([]entity.Account, error)
	SelectForLaunch(ctx context.Context, selector string) (entity.Account, error)
	SetActive(ctx context.Context, selector string) (entity.Account, error)
	ClearActive(ctx context.Context) error
	Remove(ctx context.Context, selector string) (entity.Account, error)
	RemoveProfile(ctx context.Context, selector string, deleteHome bool) (entity.Account, error)
	ReplaceImportedAuth(ctx context.Context, selector string, authJSON []byte) error
	PrepareImportedAuthRollback(ctx context.Context, accountID, id string) error
	RestoreImportedAuthRollback(ctx context.Context, accountID, id string) error
	CleanupImportedAuthRollback(ctx context.Context, accountID, id string) error
}
