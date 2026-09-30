package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

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
	if query.Profile != "" {
		matches := 0
		for _, account := range accounts {
			if account.Matches(query.Profile) {
				matches++
			}
		}
		if matches != 1 {
			return nil, fmt.Errorf("profile selector %q is missing or ambiguous", query.Profile)
		}
	}
	reports := make([]sessionmodel.Report, 0)
	for _, account := range accounts {
		if query.Profile != "" && !account.Matches(query.Profile) {
			continue
		}
		profileReports, err := service.reader.List(ctx, service.accounts.CodexHome(account.ID))
		if err != nil {
			return nil, err
		}
		for _, stored := range profileReports {
			report := sessionmodel.Report{ID: stored.ID, ThreadName: stored.ThreadName,
				UpdatedAt: stored.UpdatedAt, UpdatedUnix: stored.UpdatedUnix, CWD: stored.CWD,
				ModelProvider: stored.ModelProvider, Path: stored.Path, ParentThreadID: stored.ParentThreadID,
				Profile: account.Name, AccountID: account.ID}
			if matchesSessionQuery(report, query) {
				reports = append(reports, report)
			}
		}
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].UpdatedUnix != reports[j].UpdatedUnix {
			return reports[i].UpdatedUnix > reports[j].UpdatedUnix
		}
		if reports[i].ID != reports[j].ID {
			return reports[i].ID < reports[j].ID
		}
		return reports[i].Path < reports[j].Path
	})
	if (query.LimitSet || query.Limit > 0) && len(reports) > query.Limit {
		reports = reports[:query.Limit]
	}
	return reports, nil
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
