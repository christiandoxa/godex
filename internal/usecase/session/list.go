package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

func (service *Catalog) List(ctx context.Context, query sessionmodel.Query) ([]sessionmodel.Report, error) {
	if query.Limit < 0 {
		return nil, fmt.Errorf("session limit must not be negative")
	}
	accounts, err := service.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateProfileSelector(accounts, query.Profile); err != nil {
		return nil, err
	}
	reports, err := service.collectReports(ctx, accounts, query)
	if err != nil {
		return nil, err
	}
	sortSessionReports(reports)
	return limitSessionReports(reports, query), nil
}

func validateProfileSelector(accounts []accountentity.Account, selector string) error {
	if selector == "" {
		return nil
	}
	matches := 0
	for _, account := range accounts {
		if account.Matches(selector) {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("profile selector %q is missing or ambiguous", selector)
	}
	return nil
}

func (service *Catalog) collectReports(ctx context.Context, accounts []accountentity.Account, query sessionmodel.Query) ([]sessionmodel.Report, error) {
	reports := make([]sessionmodel.Report, 0)
	for _, account := range accounts {
		if query.Profile != "" && !account.Matches(query.Profile) {
			continue
		}
		profileReports, err := service.reader.List(ctx, service.accounts.CodexHome(account.ID))
		if err != nil {
			return nil, err
		}
		reports = append(reports, matchingReports(account, profileReports, query)...)
	}
	return reports, nil
}

func matchingReports(account accountentity.Account, stored []sessionentity.Session, query sessionmodel.Query) []sessionmodel.Report {
	reports := make([]sessionmodel.Report, 0, len(stored))
	for _, session := range stored {
		report := sessionReport(account, session)
		if matchesSessionQuery(report, query) {
			reports = append(reports, report)
		}
	}
	return reports
}

func sessionReport(account accountentity.Account, stored sessionentity.Session) sessionmodel.Report {
	return sessionmodel.Report{
		ID: stored.ID, ThreadName: stored.ThreadName,
		UpdatedAt: stored.UpdatedAt, UpdatedUnix: stored.UpdatedUnix, CWD: stored.CWD,
		ModelProvider: stored.ModelProvider, Source: stored.Source, Path: stored.Path, ParentThreadID: stored.ParentThreadID,
		Profile: account.Name, AccountID: account.ID,
	}
}

func sortSessionReports(reports []sessionmodel.Report) {
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].UpdatedUnix != reports[j].UpdatedUnix {
			return reports[i].UpdatedUnix > reports[j].UpdatedUnix
		}
		if reports[i].ID != reports[j].ID {
			return reports[i].ID < reports[j].ID
		}
		return reports[i].Path < reports[j].Path
	})
}

func limitSessionReports(reports []sessionmodel.Report, query sessionmodel.Query) []sessionmodel.Report {
	if (query.LimitSet || query.Limit > 0) && len(reports) > query.Limit {
		return reports[:query.Limit]
	}
	return reports
}

func matchesSessionQuery(report sessionmodel.Report, query sessionmodel.Query) bool {
	if query.ParentOnly && report.ParentThreadID != "" {
		return false
	}
	if query.CurrentDir != "" && !sameSessionPath(report.CWD, query.CurrentDir) {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(query.Text))
	if text == "" {
		return true
	}
	for _, value := range []string{
		report.ID, report.ThreadName, report.CWD, report.Profile,
		report.ModelProvider, report.Path,
	} {
		if strings.Contains(strings.ToLower(value), text) {
			return true
		}
	}
	return false
}

func sameSessionPath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
