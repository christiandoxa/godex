package runtime

import (
	"context"
	"errors"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

type launchAccounts interface {
	LaunchCandidates(context.Context, string) ([]accountentity.Account, error)
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

type quotaPreflight interface {
	Ready(context.Context, accountentity.Account) (bool, error)
}

type Runner struct {
	accounts launchAccounts
	process  codexProcess
	newProxy ProxyFactory
	quota    quotaPreflight
	upstream string
}

func NewRunner(accounts launchAccounts, process codexProcess, newProxy ProxyFactory) *Runner {
	return &Runner{accounts: accounts, process: process, newProxy: newProxy}
}

func (runner *Runner) SetQuotaPreflight(preflight quotaPreflight) {
	runner.quota = preflight
}

func (runner *Runner) SetUpstreamURL(upstream string) {
	if upstream != "" {
		runner.upstream = upstream
	}
}

func (runner *Runner) Run(ctx context.Context, selector string, arguments []string) (runErr error) {
	selected, exhausted, err := runner.selectForLaunch(ctx, selector)
	if err != nil {
		return err
	}
	if leases, ok := runner.accounts.(interface {
		AcquireProfiles(context.Context, []string) (func() error, error)
	}); ok {
		candidates, err := runner.accounts.LaunchCandidates(ctx, selector)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(candidates))
		for _, account := range candidates {
			ids = append(ids, account.ID)
		}
		release, err := leases.AcquireProfiles(ctx, ids)
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, release()) }()
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
	profiles, err := runner.proxyAccounts(ctx, exhausted, selector, selected.ID)
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
	runErr = proxyRunner.RunThroughProxy(ctx, home, proxy.Endpoint(), arguments)
	closeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if closeErr := proxy.Close(closeContext); runErr == nil {
		runErr = closeErr
	}
	return runErr
}

func (runner *Runner) proxyAccounts(ctx context.Context, exhausted map[string]bool, selector, selectedID string) ([]proxyconfig.Account, error) {
	accounts, err := runner.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]proxyconfig.Account, 0, len(accounts))
	for _, account := range accounts {
		if selector != "" && account.ID != selectedID {
			continue
		}
		profiles = append(profiles, proxyconfig.Account{
			ID:      account.ID,
			Home:    runner.accounts.CodexHome(account.ID),
			Enabled: account.Enabled && !exhausted[account.ID],
		})
	}
	return profiles, nil
}
