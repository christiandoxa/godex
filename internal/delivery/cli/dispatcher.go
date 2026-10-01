package cli

import (
	"context"
	"fmt"
	"io"

	accountcli "github.com/christiandoxa/godex/internal/delivery/cli/account"
	authcli "github.com/christiandoxa/godex/internal/delivery/cli/auth"
	quotacli "github.com/christiandoxa/godex/internal/delivery/cli/quota"
	runtimecli "github.com/christiandoxa/godex/internal/delivery/cli/runtime"
	sessioncli "github.com/christiandoxa/godex/internal/delivery/cli/session"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
	"github.com/christiandoxa/godex/internal/version"
)

type App struct {
	login      *authusecase.Login
	importer   *authusecase.ImportCurrent
	accounts   accountcli.Commands
	runtime    *runtimeusecase.Runner
	doctor     *runtimeusecase.Doctor
	quota      *quotausecase.Status
	nativeAuth *authusecase.Native
	sessions   *sessionusecase.Catalog
	out        io.Writer
}

func New(
	login *authusecase.Login,
	importer *authusecase.ImportCurrent,
	accounts accountcli.Commands,
	runtime *runtimeusecase.Runner,
	doctor *runtimeusecase.Doctor,
	quota *quotausecase.Status,
	stdout io.Writer,
) *App {
	return &App{
		login: login, importer: importer, accounts: accounts,
		runtime: runtime, doctor: doctor, quota: quota, out: stdout,
	}
}

func (app *App) Run(ctx context.Context, arguments []string) error {
	if len(arguments) == 0 {
		return runtimecli.Launch(ctx, app.runtime)
	}

	switch arguments[0] {
	case "login":
		if len(arguments) > 1 && arguments[1] == "status" {
			return authcli.Native(ctx, app.nativeAuth, false, arguments[2:])
		}
		return authcli.Login(ctx, app.login, app.out, arguments[1:])
	case "logout":
		return authcli.Native(ctx, app.nativeAuth, true, arguments[1:])
	case "accounts":
		return accountcli.List(ctx, app.accounts, app.out, arguments[1:])
	case "current":
		return accountcli.Current(ctx, app.accounts, app.out, arguments[1:])
	case "import-current":
		return authcli.ImportCurrent(ctx, app.importer, app.out, arguments[1:])
	case "account", "profile":
		return app.runAccountGroup(ctx, arguments[1:])
	case "use":
		return accountcli.Use(ctx, app.accounts, app.out, arguments[1:])
	case "remove":
		return accountcli.Remove(ctx, app.accounts, app.out, arguments[1:])
	case "run":
		return runtimecli.Run(ctx, app.runtime, app.sessions, arguments[1:])
	case "quota":
		if app.quota == nil {
			return fmt.Errorf("quota support is not configured")
		}
		return quotacli.Show(ctx, app.quota, app.out, arguments[1:])
	case "session":
		if app.sessions == nil {
			return fmt.Errorf("session support is not configured")
		}
		return sessioncli.Run(ctx, app.sessions, app.out, arguments[1:])
	case "doctor":
		return runtimecli.Doctor(ctx, app.doctor, app.out, arguments[1:])
	case "version", "--version", "-version":
		_, err := fmt.Fprintln(app.out, version.String())
		return err
	case "help", "--help", "-h":
		return printHelp(app.out)
	default:
		return runtimecli.Run(ctx, app.runtime, app.sessions, arguments)
	}
}

func (app *App) runAccountGroup(ctx context.Context, arguments []string) error {
	if len(arguments) > 0 && arguments[0] == "import-current" {
		return authcli.ImportCurrent(ctx, app.importer, app.out, arguments[1:])
	}
	return accountcli.Run(ctx, app.accounts, app.out, arguments)
}

func printHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `Godex manages isolated ChatGPT accounts for the official Codex CLI.

Usage:
  godex                         Launch Codex with the next managed account
  godex login [options]         Sign in with ChatGPT through Codex
  godex login status [--account SEL]
  godex logout [--account SEL]   Remove authentication, retaining native state
  godex accounts                List accounts
  godex current                 Show the active account
  godex profile import-current [name]
                               Import the current Codex ChatGPT login
  godex account use <selector>  Choose the first account for the next launch
  godex account enable/disable <sel>
                               Retain a profile while controlling eligibility
  godex account remove <sel>    Remove an account
  godex quota [--all] [--detail|--raw] [--once] [selector]
                               Show ChatGPT quota for managed accounts
  godex run [--account SEL] -- [codex args...]
  godex session list/current [--json|--id-only|--resume-command]
                               Find sessions across managed profiles
  godex session resume ID       Resume in the owning profile
  godex doctor
  godex --version
  godex <codex-subcommand> ...  Run an unknown Codex command through Godex

Run options (before the Codex command/flags):
  --account SEL                     Choose a managed account
  --web-search MODE                 disabled, cached, indexed, or live
  --rollout-budget-tokens N         Enable Codex rollout-budget reminders
  --current-time-reminder           Enable Codex current-time reminders
  --respect-system-proxy            Enable Codex system-proxy support
  --no-respect-system-proxy         Disable Codex system-proxy support

Login options:
  --name NAME      Friendly account name
  --device-auth    Use Codex device authentication
`)
	return err
}

func (app *App) SetSessions(catalog *sessionusecase.Catalog) { app.sessions = catalog }

func (app *App) SetNativeAuth(native *authusecase.Native) { app.nativeAuth = native }
