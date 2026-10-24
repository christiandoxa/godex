package session

import (
	"context"
	"os"
	"strings"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

// LastResumeSettings restores shared settings without claiming to know which
// thread native Codex will select from its provider-filtered SQLite catalog.
func (catalog *Catalog) LastResumeSettings(ctx context.Context, args []string) (sessionmodel.Report, error) {
	if strings.TrimSpace(catalog.sharedCodexHome) == "" {
		return sessionmodel.Report{}, nil
	}
	query := sessionmodel.Query{ParentOnly: true}
	if !sessionArgumentPresent(args, "--all") {
		cwd, err := os.Getwd()
		if err != nil {
			return sessionmodel.Report{}, err
		}
		query.CurrentDir = cwd
	}
	reports, err := catalog.List(ctx, query)
	if err != nil {
		return sessionmodel.Report{}, err
	}
	for _, report := range reports {
		if !sessionReportArchived(report.Path) {
			return sessionmodel.Report{ModelProvider: report.ModelProvider,
				LastModel: report.LastModel, LastReasoningEffort: report.LastReasoningEffort}, nil
		}
	}
	return sessionmodel.Report{}, nil
}
