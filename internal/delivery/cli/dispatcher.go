package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	accountcli "github.com/christiandoxa/godex/internal/delivery/cli/account"
	authcli "github.com/christiandoxa/godex/internal/delivery/cli/auth"
	pingcli "github.com/christiandoxa/godex/internal/delivery/cli/ping"
	profilecli "github.com/christiandoxa/godex/internal/delivery/cli/profile"
	quotacli "github.com/christiandoxa/godex/internal/delivery/cli/quota"
	runtimecli "github.com/christiandoxa/godex/internal/delivery/cli/runtime"
	sessioncli "github.com/christiandoxa/godex/internal/delivery/cli/session"
	updatecli "github.com/christiandoxa/godex/internal/delivery/cli/update"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
	pingusecase "github.com/christiandoxa/godex/internal/usecase/ping"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
	updateusecase "github.com/christiandoxa/godex/internal/usecase/update"
	"github.com/christiandoxa/godex/internal/version"
)

const importCurrentCommand = "import-current"

type App struct {
	login      *authusecase.Login
	importer   *authusecase.ImportCurrent
	accounts   accountcli.Commands
	runtime    *runtimeusecase.Runner
	doctor     *runtimeusecase.Doctor
	activity   *runtimeusecase.Activity
	quota      *quotausecase.Status
	redeemer   *quotausecase.Redeemer
	ping       *pingusecase.OpenAI
	updater    *updateusecase.Updater
	profiles   *profileusecase.Catalog
	nativeAuth *authusecase.Native
	sessions   *sessionusecase.Catalog
	out        io.Writer
	errOut     io.Writer
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
		runtime: runtime, doctor: doctor, quota: quota, out: stdout, errOut: io.Discard,
	}
}

func (app *App) Run(ctx context.Context, arguments []string) error {
	app.showUpdateNotice(ctx, arguments)
	if len(arguments) == 0 {
		return app.runRuntime(ctx, nil)
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
		if app.profiles != nil {
			return profilecli.Run(ctx, app.profiles, app.out, []string{"current"})
		}
		return accountcli.Current(ctx, app.accounts, app.out, arguments[1:])
	case "import-current":
		return authcli.ImportCurrent(ctx, app.importer, app.out, arguments[1:])
	case "account":
		return app.runAccountGroup(ctx, arguments[1:])
	case "profile":
		return app.runProfileGroup(ctx, arguments[1:])
	case "use":
		if app.profiles != nil {
			return profilecli.Run(ctx, app.profiles, app.out, append([]string{"use"}, arguments[1:]...))
		}
		return accountcli.Use(ctx, app.accounts, app.out, arguments[1:])
	case "remove":
		return accountcli.Remove(ctx, app.accounts, app.out, arguments[1:])
	case "run":
		return app.runRuntime(ctx, arguments[1:])
	case "quota":
		if app.quota == nil {
			return fmt.Errorf("quota support is not configured")
		}
		return quotacli.Show(ctx, app.quota, app.out, arguments[1:])
	case "redeem":
		return quotacli.Redeem(ctx, app.redeemer, app.out, arguments[1:])
	case "ping":
		return pingcli.Run(ctx, app.ping, app.out, arguments[1:])
	case "update":
		return updatecli.Run(ctx, app.updater, app.out, app.errOut, arguments[1:])
	case "session":
		if app.sessions == nil {
			return fmt.Errorf("session support is not configured")
		}
		return sessioncli.Run(ctx, app.sessions, app.out, arguments[1:])
	case "info":
		return runtimecli.Info(ctx, app.activity, app.out, arguments[1:])
	case "status":
		return runtimecli.Status(ctx, app.activity, app.out, arguments[1:])
	case "log":
		return runtimecli.Log(ctx, app.activity, app.out, arguments[1:])
	case "doctor":
		return runtimecli.Doctor(ctx, app.doctor, app.out, arguments[1:])
	case "version", "--version", "-version":
		_, err := fmt.Fprintln(app.out, version.String())
		return err
	case "help", "--help", "-h":
		return printHelp(app.out)
	default:
		return app.runRuntime(ctx, arguments)
	}
}

func (app *App) showUpdateNotice(ctx context.Context, arguments []string) {
	if app.updater == nil || !shouldShowUpdateNotice(arguments) {
		return
	}
	_ = updatecli.Notice(ctx, app.updater, app.errOut)
}

func shouldShowUpdateNotice(arguments []string) bool {
	if len(arguments) == 0 {
		return true
	}
	switch arguments[0] {
	case "info", "log", "ping", "update", "version", "--version", "-version", "help", "--help", "-h":
		return false
	case "quota":
		for _, argument := range arguments[1:] {
			if argument == "--raw" {
				return false
			}
		}
	case "doctor":
		for _, argument := range arguments[1:] {
			if argument == "--json" || argument == "--bundle" || strings.HasPrefix(argument, "--bundle=") {
				return false
			}
		}
	}
	return true
}

func (app *App) runRuntime(ctx context.Context, arguments []string) error {
	if app.profiles != nil {
		return runtimecli.RunProfiles(ctx, app.runtime, app.sessions, app.profiles, arguments)
	}
	return runtimecli.Run(ctx, app.runtime, app.sessions, arguments)
}

func (app *App) runAccountGroup(ctx context.Context, arguments []string) error {
	if len(arguments) > 0 && arguments[0] == importCurrentCommand {
		return authcli.ImportCurrent(ctx, app.importer, app.out, arguments[1:])
	}
	return accountcli.Run(ctx, app.accounts, app.out, arguments)
}

func (app *App) runProfileGroup(ctx context.Context, arguments []string) error {
	if len(arguments) > 0 && arguments[0] == importCurrentCommand {
		return authcli.ImportCurrent(ctx, app.importer, app.out, arguments[1:])
	}
	if app.profiles == nil {
		return accountcli.Run(ctx, app.accounts, app.out, arguments)
	}
	return profilecli.Run(ctx, app.profiles, app.out, arguments)
}

func printHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `Godex manages isolated ChatGPT accounts for the official Codex CLI.

Usage:
  godex                         Launch Codex with the next managed account
  godex login [options]         Sign in with ChatGPT through Codex
  godex login status [--account SEL]
  godex logout [--account SEL]   Remove authentication, retaining native state
  godex accounts                List account identities
  godex current                 Show the active profile and CODEX_HOME
  godex profile add NAME [options]
                               Add managed or external CODEX_HOME profile
  godex profile list            List configured profiles
  godex profile export [options] [PATH]
                               Export a Prodex-compatible profile bundle
  godex profile import PATH    Import a Prodex-compatible profile bundle
  godex profile use --profile NAME
                               Set the active profile
  godex profile remove NAME [--delete-home]
  godex profile import-current [name]
                               Import the current Codex ChatGPT login
  godex account use <selector>  Choose the first account for account rotation
  godex account enable/disable <sel>
                               Retain a profile while controlling eligibility
  godex account remove <sel>    Remove an account
  godex quota [-p NAME] [--all] [--auth AUTH] [--provider PROVIDER]
              [--detail] [--raw] [--once] [--base-url URL]
                               Watch or snapshot filtered profile quota
  godex redeem PROFILE [-y|--yes] [--base-url URL] [--no-proxy]
                               Redeem one OpenAI reset credit manually
  godex ping openai [-p NAME] [--model MODEL] [--base-url URL] [--no-proxy] [--json]
                               Run a cost-bearing OpenAI application diagnostic
  godex update                  Update from the latest verified GitHub release
  godex run [--account SEL] -- [codex args...]
  godex session list/current [--json|--id-only|--resume-command]
                               Find sessions across managed profiles
  godex session resume ID       Resume in the owning profile
  godex info [--json] [--tokens]
  godex status [--once] [--interval N]
  godex log [stream|last|upstream] [--json]
  godex doctor [--quota] [--runtime] [--install] [--tail-bytes BYTES]
               [--json] [--bundle [PATH] --redacted]
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

func (app *App) SetProfiles(catalog *profileusecase.Catalog) { app.profiles = catalog }

func (app *App) SetRedeemer(redeemer *quotausecase.Redeemer) { app.redeemer = redeemer }

func (app *App) SetPing(ping *pingusecase.OpenAI) { app.ping = ping }

func (app *App) SetUpdate(updater *updateusecase.Updater, stderr io.Writer) {
	app.updater = updater
	if stderr != nil {
		app.errOut = stderr
	}
}

func (app *App) SetNativeAuth(native *authusecase.Native) { app.nativeAuth = native }

func (app *App) SetActivity(activity *runtimeusecase.Activity) { app.activity = activity }
