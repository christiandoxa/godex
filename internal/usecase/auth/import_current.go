package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	authmodel "github.com/christiandoxa/godex/internal/model/auth"
)

const defaultImportCurrentName = "default"

type importCurrentAccounts interface {
	CreateStagedHome() (string, error)
	RemoveStagedHome(string) error
	CommitImportCurrent(context.Context, accountentity.Account, string) (accountentity.Account, error)
	List(context.Context) ([]accountentity.Account, error)
}

type importCurrentCodex interface {
	StageImportCurrentAuth(context.Context, string, string, bool) (authmodel.ImportCurrentIdentity, error)
	CompleteImportCurrentHome(context.Context, string, string) error
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

func (importer *ImportCurrent) Run(
	ctx context.Context,
	request authmodel.ImportCurrentRequest,
) (response authmodel.ImportCurrentResponse, err error) {
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		request.Name = defaultImportCurrentName
	}
	existing, err := importer.accounts.List(ctx)
	if err != nil {
		return authmodel.ImportCurrentResponse{}, err
	}
	for _, account := range existing {
		if account.Name == request.Name {
			return authmodel.ImportCurrentResponse{}, fmt.Errorf(
				"profile %q already exists", request.Name,
			)
		}
	}

	stagedHome, err := importer.accounts.CreateStagedHome()
	if err != nil {
		return authmodel.ImportCurrentResponse{}, err
	}
	defer func() {
		if cleanupErr := importer.accounts.RemoveStagedHome(stagedHome); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove staged Codex home: %w", cleanupErr))
		}
	}()

	result, err := importer.codex.StageImportCurrentAuth(
		ctx, importer.sourceHome, stagedHome, request.Insecure,
	)
	if err != nil {
		return authmodel.ImportCurrentResponse{}, err
	}
	identity := accountentity.Identity{
		Email: result.Email, ChatGPTAccountID: result.ChatGPTAccountID,
	}
	duplicateIdentity := false
	for _, account := range existing {
		if account.SameIdentity(identity) {
			duplicateIdentity = true
			break
		}
	}
	if !duplicateIdentity {
		if err := importer.codex.CompleteImportCurrentHome(ctx, importer.sourceHome, stagedHome); err != nil {
			return authmodel.ImportCurrentResponse{}, err
		}
	}

	candidate, err := accountentity.NewAccount(identity, request.Name, importer.now())
	if err != nil {
		return authmodel.ImportCurrentResponse{}, err
	}
	account, err := importer.accounts.CommitImportCurrent(ctx, candidate, stagedHome)
	if err != nil {
		return authmodel.ImportCurrentResponse{}, err
	}
	return authmodel.ImportCurrentResponse{
		ID: account.ID, Name: account.Name, Email: account.Email,
	}, nil
}
