package session

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

type accountStore interface {
	List(context.Context) ([]accountentity.Account, error)
	CodexHome(string) string
}

type reader interface {
	List(context.Context, string) ([]sessionentity.Session, error)
}

type launcher interface {
	Run(context.Context, string, []string) error
	RunLocal(context.Context, string, []string) error
}

type Catalog struct {
	accounts accountStore
	reader   reader
	launcher launcher
}

func NewCatalog(accounts accountStore, reader reader, launcher launcher) *Catalog {
	return &Catalog{accounts: accounts, reader: reader, launcher: launcher}
}
