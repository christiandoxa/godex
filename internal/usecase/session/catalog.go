package session

import (
	"context"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

type accountStore interface {
	List(context.Context) ([]accountentity.Account, error)
	CodexHome(string) string
}

type reader interface {
	List(context.Context, string) ([]sessionentity.Session, error)
}

type profileSource interface {
	SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error)
}

type ReportLauncher interface {
	RunSessionReport(context.Context, sessionmodel.Report, []string, bool) error
}

type Launcher interface {
	Run(context.Context, string, []string) error
	RunLocal(context.Context, string, []string) error
	RunSession(context.Context, string, string, []string) error
}

// A recovery-aware launcher may observe a verified post-child failure
// and decide whether an already-resolved session can safely resume.
type sessionChildRecoveryLauncher interface {
	RunSessionReportWithRecovery(
		context.Context, sessionmodel.Report, []string, bool,
		func(context.Context, string) error,
	) error
}

type Catalog struct {
	accounts        accountStore
	reader          reader
	launcher        Launcher
	ownerLookup     func(context.Context, string) (string, error)
	bindingForget   func(context.Context, string) error
	profiles        profileSource
	sharedCodexHome string
}

func NewCatalog(accounts accountStore, reader reader, launcher Launcher) *Catalog {
	return &Catalog{accounts: accounts, reader: reader, launcher: launcher}
}

func (catalog *Catalog) SetOwnerLookup(lookup func(context.Context, string) (string, error)) {
	catalog.ownerLookup = lookup
}

func (catalog *Catalog) SetBindingForget(forget func(context.Context, string) error) {
	catalog.bindingForget = forget
}

func (catalog *Catalog) SetProfiles(profiles profileSource) { catalog.profiles = profiles }

func (catalog *Catalog) SetSharedCodexHome(home string) { catalog.sharedCodexHome = home }
