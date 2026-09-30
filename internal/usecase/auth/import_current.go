package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type importCurrentAccounts interface {
	CreateStagedHome() (string, error)
	RemoveStagedHome(string) error
	CommitLogin(context.Context, accountentity.Account, string, bool) (accountentity.Account, error)
}

type importCurrentCodex interface {
	ImportCurrent(context.Context, string, string) (accountentity.Identity, error)
}

type ImportCurrent struct {
	accounts   importCurrentAccounts
	codex      importCurrentCodex
	sourceHome string
	now        func() time.Time
}

func NewImportCurrent(accounts importCurrentAccounts, codex importCurrentCodex, sourceHome string) *ImportCurrent {
	return &ImportCurrent{accounts: accounts, codex: codex, sourceHome: sourceHome, now: time.Now}
}

func (importer *ImportCurrent) Run(ctx context.Context, name string) (account accountentity.Account, err error) {
	stagedHome, err := importer.accounts.CreateStagedHome()
	if err != nil {
		return accountentity.Account{}, err
	}
	defer func() {
		if cleanupErr := importer.accounts.RemoveStagedHome(stagedHome); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove staged Codex home: %w", cleanupErr))
		}
	}()

	identity, err := importer.codex.ImportCurrent(ctx, importer.sourceHome, stagedHome)
	if err != nil {
		return accountentity.Account{}, err
	}
	candidate, err := accountentity.NewAccount(identity, name, importer.now())
	if err != nil {
		return accountentity.Account{}, err
	}
	return importer.accounts.CommitLogin(ctx, candidate, stagedHome, name != "")
}
