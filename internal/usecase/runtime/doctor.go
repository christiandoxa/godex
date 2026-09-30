package runtime

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	accountmodel "github.com/christiandoxa/godex/internal/model/account"
)

type doctorAccounts interface {
	Prepare() error
	Root() string
	List(context.Context) ([]accountentity.Account, error)
}

type versionedCodex interface {
	Version(context.Context) (string, error)
	CheckProxySupport(context.Context) error
}

type Doctor struct {
	accounts doctorAccounts
	codex    versionedCodex
}

func NewDoctor(accounts doctorAccounts, codex versionedCodex) *Doctor {
	return &Doctor{accounts: accounts, codex: codex}
}

func (doctor *Doctor) Run(ctx context.Context) (accountmodel.DoctorReport, error) {
	if err := doctor.accounts.Prepare(); err != nil {
		return accountmodel.DoctorReport{}, err
	}
	if err := doctor.codex.CheckProxySupport(ctx); err != nil {
		return accountmodel.DoctorReport{}, err
	}
	codexVersion, err := doctor.codex.Version(ctx)
	if err != nil {
		return accountmodel.DoctorReport{}, err
	}
	accounts, err := doctor.accounts.List(ctx)
	if err != nil {
		return accountmodel.DoctorReport{}, err
	}
	enabled := 0
	for _, account := range accounts {
		if account.Enabled {
			enabled++
		}
	}
	return accountmodel.DoctorReport{
		GodexHome:    doctor.accounts.Root(),
		CodexVersion: codexVersion,
		AccountCount: len(accounts),
		EnabledCount: enabled,
	}, nil
}
