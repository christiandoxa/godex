package cli

import (
	"context"
	"fmt"
	"io"

	accountcli "github.com/christiandoxa/godex/internal/delivery/cli/account"
	authcli "github.com/christiandoxa/godex/internal/delivery/cli/auth"
	runtimecli "github.com/christiandoxa/godex/internal/delivery/cli/runtime"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	"github.com/christiandoxa/godex/internal/version"
)

type App struct {
	login    *authusecase.Login
	accounts accountcli.Commands
	runtime  *runtimeusecase.Runner
	doctor   *runtimeusecase.Doctor
	out      io.Writer
}

func New(login *authusecase.Login, accounts accountcli.Commands, runtime *runtimeusecase.Runner, doctor *runtimeusecase.Doctor, stdout io.Writer) *App {
	return &App{login: login, accounts: accounts, runtime: runtime, doctor: doctor, out: stdout}
}

func (app *App) Run(ctx context.Context, arguments []string) error {
	if len(arguments) == 0 {
		return runtimecli.Launch(ctx, app.runtime)
	}

	switch arguments[0] {
	case "login":
		return authcli.Login(ctx, app.login, app.out, arguments[1:])
	case "accounts":
		return accountcli.List(ctx, app.accounts, app.out, arguments[1:])
	case "account":
		return accountcli.Run(ctx, app.accounts, app.out, arguments[1:])
	case "use":
		return accountcli.Use(ctx, app.accounts, app.out, arguments[1:])
	case "remove":
		return accountcli.Remove(ctx, app.accounts, app.out, arguments[1:])
	case "run":
		return runtimecli.Run(ctx, app.runtime, arguments[1:])
	case "doctor":
		return runtimecli.Doctor(ctx, app.doctor, app.out, arguments[1:])
	case "version", "--version", "-version":
		_, err := fmt.Fprintln(app.out, version.String())
		return err
	case "help", "--help", "-h":
		return printHelp(app.out)
	default:
		return fmt.Errorf("unknown command %q; run `godex help`", arguments[0])
	}
}

func printHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, `Godex manages isolated ChatGPT accounts for the official Codex CLI.

Usage:
  godex                         Launch Codex with the next managed account
  godex login [options]         Sign in with ChatGPT through Codex
  godex accounts                List accounts
  godex account use <selector>  Choose the first account for the next launch
  godex account remove <sel>    Remove an account
  godex run [--account SEL] -- [codex args...]
  godex doctor
  godex --version

Login options:
  --name NAME      Friendly account name
  --device-auth    Use Codex device authentication
`)
	return err
}
