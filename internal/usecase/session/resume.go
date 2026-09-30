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

func (service *Catalog) Resume(ctx context.Context, selector string) error {
	if service.launcher == nil {
		return fmt.Errorf("session resume launcher is not configured")
	}
	report, err := service.Resolve(ctx, selector)
	if err != nil {
		return err
	}
	return service.launcher.Run(ctx, report.AccountID, []string{"resume", report.ID})
}
