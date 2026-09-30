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
		return exact[0], nil
	}
	if len(exact) > 1 {
		return sessionmodel.Report{}, fmt.Errorf("session id %q is ambiguous", selector)
	}
	if len(prefix) == 1 {
		return prefix[0], nil
	}
	if len(prefix) > 1 {
		return sessionmodel.Report{}, fmt.Errorf("session id prefix %q is ambiguous", selector)
	}
	return sessionmodel.Report{}, fmt.Errorf("session %q was not found", selector)
}

func (catalog *Catalog) Resume(ctx context.Context, selector string) error {
	return catalog.ResumeArguments(ctx, "", selector, 1, []string{"resume", selector})
}

func (catalog *Catalog) ResumeArguments(ctx context.Context, accountSelector, selector string, idIndex int, arguments []string) error {
	if catalog.launcher == nil {
		return fmt.Errorf("session resume launcher is not configured")
	}
	report, err := catalog.Resolve(ctx, selector)
	if err != nil {
		return err
	}
	if accountSelector != "" {
		accounts, err := catalog.accounts.List(ctx)
		if err != nil {
			return err
		}
		matches := 0
		for _, account := range accounts {
			if account.Matches(accountSelector) {
				matches++
				if account.ID != report.AccountID {
					return fmt.Errorf("selected account does not own session %q", selector)
				}
			}
		}
		if matches != 1 {
			return fmt.Errorf("account selector is missing or ambiguous")
		}
	}
	if idIndex < 0 || idIndex >= len(arguments) {
		return fmt.Errorf("session argument index is invalid")
	}
	args := append([]string(nil), arguments...)
	args[idIndex] = report.ID
	switch args[0] {
	case "delete", "archive", "unarchive":
		return catalog.launcher.RunLocal(ctx, report.AccountID, args)
	}
	return catalog.launcher.Run(ctx, report.AccountID, args)
}
