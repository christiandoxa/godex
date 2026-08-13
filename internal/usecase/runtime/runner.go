package runtime

import (
	"context"
	"errors"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

type launchAccounts interface {
	SelectForLaunch(context.Context, string) (accountentity.Account, error)
	List(context.Context) ([]accountentity.Account, error)
	CodexHome(string) string
}

type codexProcess interface {
	Run(context.Context, string, []string) error
}

type proxyCodex interface {
	codexProcess
	CheckProxySupport(context.Context) error
	RunThroughProxy(context.Context, string, string, []string) error
}

type Runner struct {
	accounts launchAccounts
	process  codexProcess
	newProxy ProxyFactory
	upstream string
}

func NewRunner(accounts launchAccounts, process codexProcess, newProxy ProxyFactory) *Runner {
	return &Runner{accounts: accounts, process: process, newProxy: newProxy}
}

func (runner *Runner) SetUpstreamURL(upstream string) {
	if upstream != "" {
		runner.upstream = upstream
	}
}

func (runner *Runner) Run(ctx context.Context, selector string, arguments []string) error {
	selected, err := runner.accounts.SelectForLaunch(ctx, selector)
	if err != nil {
		return err
	}
	home := runner.accounts.CodexHome(selected.ID)
	proxyRunner, ok := runner.process.(proxyCodex)
	if !ok {
		return runner.process.Run(ctx, home, arguments)
	}
	if runner.newProxy == nil {
		return errors.New("runtime proxy factory is not configured")
	}
	if err := proxyRunner.CheckProxySupport(ctx); err != nil {
		return err
	}
	// ponytail: snapshot account metadata per launch. Add an owned refresh path
	// if live state changes must affect a running proxy.
	profiles, err := runner.proxyAccounts(ctx)
	if err != nil {
		return err
	}
	proxy, err := runner.newProxy(proxyconfig.Config{
		UpstreamURL:      runner.upstream,
		PreferredAccount: selected.ID,
		Accounts: func(ctx context.Context) ([]proxyconfig.Account, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return append([]proxyconfig.Account(nil), profiles...), nil
		},
	})
	if err != nil {
		return err
	}
	if err := proxy.Start(); err != nil {
		return err
	}
	runErr := proxyRunner.RunThroughProxy(ctx, home, proxy.Endpoint(), arguments)
	closeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if closeErr := proxy.Close(closeContext); runErr == nil {
		runErr = closeErr
	}
	return runErr
}

func (runner *Runner) proxyAccounts(ctx context.Context) ([]proxyconfig.Account, error) {
	accounts, err := runner.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]proxyconfig.Account, 0, len(accounts))
	for _, account := range accounts {
		profiles = append(profiles, proxyconfig.Account{
			ID: account.ID, Home: runner.accounts.CodexHome(account.ID), Enabled: account.Enabled,
		})
	}
	return profiles, nil
}
