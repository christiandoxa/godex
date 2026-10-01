package quota

import (
	"context"
	"errors"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type accountStore interface {
	List(context.Context) ([]accountentity.Account, error)
	Current(context.Context) (accountentity.Account, error)
	Resolve(context.Context, string) (accountentity.Account, error)
	CodexHome(string) string
}

type usageGateway interface {
	Fetch(context.Context, string) (quotamodel.Usage, error)
}

type rawUsageGateway interface {
	FetchRaw(context.Context, string) ([]byte, error)
}

type overrideUsageGateway interface {
	FetchAt(context.Context, string, string) (quotamodel.Usage, error)
}

type rawOverrideUsageGateway interface {
	FetchRawAt(context.Context, string, string) ([]byte, error)
}

type Options struct {
	All      bool
	Selector string
	BaseURL  string
}

type Status struct {
	accounts accountStore
	usage    usageGateway
	now      func() time.Time
}

func NewStatus(accounts accountStore, usage usageGateway) *Status {
	return &Status{accounts: accounts, usage: usage, now: time.Now}
}

func (status *Status) Raw(ctx context.Context, selector, baseURL string) ([]byte, error) {
	var account accountentity.Account
	var err error
	if selector != "" {
		account, err = status.accounts.Resolve(ctx, selector)
	} else {
		account, err = status.accounts.Current(ctx)
	}
	if err != nil {
		return nil, err
	}
	home := status.accounts.CodexHome(account.ID)
	if baseURL != "" {
		raw, ok := status.usage.(rawOverrideUsageGateway)
		if !ok {
			return nil, errors.New("quota base URL override is not supported")
		}
		return raw.FetchRawAt(ctx, home, baseURL)
	}
	raw, ok := status.usage.(rawUsageGateway)
	if !ok {
		return nil, errors.New("raw quota output is not supported")
	}
	return raw.FetchRaw(ctx, home)
}

func (status *Status) Ready(ctx context.Context, account accountentity.Account) (bool, error) {
	availability, err := status.Availability(ctx, account)
	return availability.Ready, err
}

func (status *Status) Run(ctx context.Context, options Options) ([]quotamodel.Report, error) {
	accounts, err := status.selectedAccounts(ctx, options)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, errors.New("no managed accounts; run `godex login` or `godex profile import-current`")
	}
	current, _ := status.accounts.Current(ctx)
	reports := make([]quotamodel.Report, 0, len(accounts))
	for _, account := range accounts {
		report := quotamodel.Report{
			AccountName: account.Name,
			Email:       account.Email,
			Active:      account.ID == current.ID,
			Enabled:     account.Enabled,
		}
		if account.Enabled {
			report.Usage, report.Err = status.fetchUsage(ctx, account, options.BaseURL)
		}
		report.State = quotaState(report, status.now())
		reports = append(reports, report)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return reports, nil
}

func (status *Status) fetchUsage(ctx context.Context, account accountentity.Account, baseURL string) (quotamodel.Usage, error) {
	home := status.accounts.CodexHome(account.ID)
	if baseURL == "" {
		return status.usage.Fetch(ctx, home)
	}
	override, ok := status.usage.(overrideUsageGateway)
	if !ok {
		return quotamodel.Usage{}, errors.New("quota base URL override is not supported")
	}
	return override.FetchAt(ctx, home, baseURL)
}

func (status *Status) selectedAccounts(ctx context.Context, options Options) ([]accountentity.Account, error) {
	if options.All {
		if options.Selector != "" {
			return nil, errors.New("quota selector cannot be combined with --all")
		}
		return status.accounts.List(ctx)
	}
	if options.Selector != "" {
		account, err := status.accounts.Resolve(ctx, options.Selector)
		if err != nil {
			return nil, err
		}
		return []accountentity.Account{account}, nil
	}
	account, err := status.accounts.Current(ctx)
	if err != nil {
		return nil, err
	}
	return []accountentity.Account{account}, nil
}

func quotaState(report quotamodel.Report, now time.Time) string {
	if !report.Enabled {
		return "disabled"
	}
	if report.Err != nil {
		return "error"
	}
	usage := report.Usage
	if usage.Allowed != nil && !*usage.Allowed {
		return "exhausted"
	}
	if usage.LimitReached != nil && *usage.LimitReached {
		return "exhausted"
	}
	if windowExhausted(usage.Primary, now) || windowExhausted(usage.Secondary, now) {
		return "exhausted"
	}
	return "ready"
}

func windowExhausted(window *quotamodel.Window, now time.Time) bool {
	if window == nil || window.UsedPercent == nil || *window.UsedPercent < 100 {
		return false
	}
	return window.ResetAt == nil || *window.ResetAt > now.Unix()
}
