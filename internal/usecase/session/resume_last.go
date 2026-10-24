package session

import (
	"context"
	"os"
	"strings"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

// NativeResumeSettings restores shared settings without claiming to know which
// thread native Codex will select from its provider-filtered SQLite catalog.
func (catalog *Catalog) NativeResumeSettings(ctx context.Context, input sessionmodel.Launch) (sessionmodel.Report, error) {
	if strings.TrimSpace(catalog.sharedCodexHome) == "" || input.SessionSelector == "" {
		return sessionmodel.Report{}, nil
	}
	query := sessionmodel.Query{ParentOnly: input.SessionSelector == "--last"}
	if query.ParentOnly && !sessionArgumentPresent(input.Arguments, "--all") {
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
	var report sessionmodel.Report
	if input.SessionSelector == "--last" {
		for _, candidate := range reports {
			if !sessionReportArchived(candidate.Path) {
				report = candidate
				break
			}
		}
	} else {
		report, err = resolveSessionID(input.SessionSelector, reports)
		if _, missing := err.(missingSessionError); !missing && err != nil {
			return sessionmodel.Report{}, err
		}
	}
	return sessionmodel.Report{ModelProvider: report.ModelProvider,
		LastModel: report.LastModel, LastReasoningEffort: report.LastReasoningEffort}, nil
}
