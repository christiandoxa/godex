package session

import (
	"context"
	"fmt"
	"os"
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
	names := make([]sessionmodel.Report, 0, 1)
	for _, report := range reports {
		if strings.EqualFold(report.ID, selector) {
			exact = append(exact, report)
			continue
		}
		if strings.HasPrefix(strings.ToLower(report.ID), strings.ToLower(selector)) {
			prefix = append(prefix, report)
		}
		if !sessionReportArchived(report.Path) && sessionReportSourceAllowed(report, false) &&
			sessionDisplayLabel(report) == selector {
			names = append(names, report)
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
	if len(names) == 1 {
		return service.withOwner(ctx, names[0])
	}
	if len(names) > 1 {
		return sessionmodel.Report{}, fmt.Errorf("session name %q is ambiguous", selector)
	}
	return sessionmodel.Report{}, fmt.Errorf("session %q was not found", selector)
}

func (catalog *Catalog) Resume(ctx context.Context, selector string) error {
	return catalog.ResumeArguments(ctx, sessionmodel.Launch{SessionSelector: selector, IDIndex: 1, Arguments: []string{"resume", selector}})
}

func (catalog *Catalog) ResumeArguments(ctx context.Context, input sessionmodel.Launch) error {
	return catalog.ResumeArgumentsWithLauncher(ctx, input, catalog.launcher)
}

func (catalog *Catalog) ResolveArguments(
	ctx context.Context,
	input sessionmodel.Launch,
) (sessionmodel.Report, []string, error) {
	var report sessionmodel.Report
	var err error
	if input.SessionSelector == "--last" {
		report, err = catalog.resolveLast(ctx, input.Arguments)
	} else {
		report, err = catalog.Resolve(ctx, input.SessionSelector)
	}
	if err != nil {
		return sessionmodel.Report{}, nil, err
	}
	if err := catalog.validateAccountOwner(ctx, input.AccountSelector, report); err != nil {
		return sessionmodel.Report{}, nil, err
	}
	args, err := resolvedSessionArguments(input, report.ID)
	if err != nil {
		return sessionmodel.Report{}, nil, err
	}
	return report, args, nil
}

func (catalog *Catalog) ResumeArgumentsWithLauncher(
	ctx context.Context,
	input sessionmodel.Launch,
	launcher Launcher,
) error {
	if launcher == nil {
		return fmt.Errorf("session resume launcher is not configured")
	}
	report, args, err := catalog.ResolveArguments(ctx, input)
	if err != nil {
		return err
	}
	var launchErr error
	if aware, ok := launcher.(ReportLauncher); ok {
		launchErr = aware.RunSessionReport(ctx, report, args, input.Local)
	} else if input.Local {
		launchErr = launcher.RunLocal(ctx, report.AccountID, args)
	} else {
		launchErr = launcher.RunSession(ctx, report.AccountID, report.UpstreamAccountID, args)
	}
	if launchErr != nil {
		return launchErr
	}
	if input.Delete && catalog.bindingForget != nil {
		return catalog.bindingForget(ctx, report.ID)
	}
	return nil
}

func (catalog *Catalog) resolveLast(ctx context.Context, args []string) (sessionmodel.Report, error) {
	query := sessionmodel.Query{}
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
	includeNonInteractive := sessionArgumentPresent(args, "--include-non-interactive")
	for _, report := range reports {
		if sessionReportArchived(report.Path) || !sessionReportSourceAllowed(report, includeNonInteractive) {
			continue
		}
		return catalog.withOwner(ctx, report)
	}
	return sessionmodel.Report{}, fmt.Errorf("no resumable session was found")
}

func sessionDisplayLabel(report sessionmodel.Report) string {
	if name := strings.TrimSpace(report.ThreadName); name != "" {
		return name
	}
	return strings.TrimSpace(report.Preview)
}

func sessionArgumentPresent(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func sessionReportSourceAllowed(report sessionmodel.Report, includeNonInteractive bool) bool {
	source := strings.ToLower(strings.TrimSpace(report.Source))
	switch source {
	case "", "cli", "vscode":
		return true
	case "exec", "mcp":
		return includeNonInteractive
	default:
		return false
	}
}

func sessionReportArchived(path string) bool {
	for _, component := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if component == "archived_sessions" {
			return true
		}
	}
	return false
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
