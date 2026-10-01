package session

import (
	"context"
	"fmt"
	"strings"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

func (service *Catalog) Resolve(ctx context.Context, selector string) (sessionmodel.Report, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return sessionmodel.Report{}, fmt.Errorf("session id is required")
	}
	reports, err := service.List(ctx, sessionmodel.Query{})
	if err != nil {
		return sessionmodel.Report{}, err
	}
	exact := make([]sessionmodel.Report, 0, 1)
	prefix := make([]sessionmodel.Report, 0, 1)
	for _, report := range reports {
		if strings.EqualFold(report.ID, selector) {
			exact = append(exact, report)
			continue
		}
		if strings.HasPrefix(strings.ToLower(report.ID), strings.ToLower(selector)) {
			prefix = append(prefix, report)
		}
	}
	if len(exact) == 1 {
		return service.withOwner(ctx, exact[0])
	}
	if len(exact) > 1 {
		return sessionmodel.Report{}, fmt.Errorf("session id %q is ambiguous", selector)
	}
	if len(prefix) == 1 {
		return service.withOwner(ctx, prefix[0])
	}
	if len(prefix) > 1 {
		return sessionmodel.Report{}, fmt.Errorf("session id prefix %q is ambiguous", selector)
	}
	return sessionmodel.Report{}, fmt.Errorf("session %q was not found", selector)
}

func (catalog *Catalog) Resume(ctx context.Context, selector string) error {
	return catalog.ResumeArguments(ctx, sessionmodel.Launch{SessionSelector: selector, IDIndex: 1, Arguments: []string{"resume", selector}})
}

func (catalog *Catalog) ResumeArguments(ctx context.Context, input sessionmodel.Launch) error {
	if catalog.launcher == nil {
		return fmt.Errorf("session resume launcher is not configured")
	}
	report, err := catalog.Resolve(ctx, input.SessionSelector)
	if err != nil {
		return err
	}
	if err := catalog.validateAccountOwner(ctx, input.AccountSelector, report); err != nil {
		return err
	}
	args, err := resolvedSessionArguments(input, report.ID)
	if err != nil {
		return err
	}
	if input.Local {
		return catalog.launcher.RunLocal(ctx, report.AccountID, args)
	}
	return catalog.launcher.RunSession(ctx, report.AccountID, report.UpstreamAccountID, args)
}

func (catalog *Catalog) validateAccountOwner(ctx context.Context, selector string, report sessionmodel.Report) error {
	if selector == "" {
		return nil
	}
	accounts, err := catalog.accounts.List(ctx)
	if err != nil {
		return err
	}
	matches := 0
	for _, account := range accounts {
		if !account.Matches(selector) {
			continue
		}
		matches++
		if account.ID != report.AccountID {
			return fmt.Errorf("selected account does not own session %q", report.ID)
		}
	}
	if matches != 1 {
		return fmt.Errorf("account selector is missing or ambiguous")
	}
	return nil
}

func resolvedSessionArguments(input sessionmodel.Launch, sessionID string) ([]string, error) {
	if input.IDIndex < 0 || input.IDIndex >= len(input.Arguments) {
		return nil, fmt.Errorf("session argument index is invalid")
	}
	args := append([]string(nil), input.Arguments...)
	args[input.IDIndex] = input.IDPrefix + sessionID
	return args, nil
}

func (catalog *Catalog) withOwner(ctx context.Context, report sessionmodel.Report) (sessionmodel.Report, error) {
	report.UpstreamAccountID = report.AccountID
	if catalog.ownerLookup != nil {
		owner, err := catalog.ownerLookup(ctx, report.ID)
		if err != nil {
			return sessionmodel.Report{}, err
		}
		if owner != "" {
			report.UpstreamAccountID = owner
		}
	}
	return report, nil
}
