package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

func (service *Catalog) List(ctx context.Context, query sessionmodel.Query) ([]sessionmodel.Report, error) {
	if query.Limit < 0 {
		return nil, fmt.Errorf("session limit must not be negative")
	}
	homes, err := service.sessionProfileHomes(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateProfileSelector(homes, query.Profile); err != nil {
		return nil, err
	}
	var reports []sessionmodel.Report
	if strings.TrimSpace(service.sharedCodexHome) != "" {
		reports, err = service.collectSharedReports(ctx, homes, query)
	} else {
		reports, err = service.collectReports(ctx, homes, query)
	}
	if err != nil {
		return nil, err
	}
	sortSessionReports(reports)
	return limitSessionReports(reports, query), nil
}

func (service *Catalog) sessionProfileHomes(ctx context.Context) ([]sessionmodel.ProfileHome, error) {
	if service.profiles != nil {
		return service.profiles.SessionProfiles(ctx)
	}
	accounts, err := service.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	homes := make([]sessionmodel.ProfileHome, 0, len(accounts))
	for _, account := range accounts {
		homes = append(homes, sessionmodel.ProfileHome{
			Name: account.Name, AccountID: account.ID, Email: account.Email,
			CodexHome: service.accounts.CodexHome(account.ID), Enabled: account.Enabled,
			Provider: account.ProviderKind,
		})
	}
	return homes, nil
}

func validateProfileSelector(homes []sessionmodel.ProfileHome, selector string) error {
	if selector == "" {
		return nil
	}
	matches := 0
	for _, home := range homes {
		if sessionProfileMatches(home, selector) {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("profile selector %q is missing or ambiguous", selector)
	}
	return nil
}

func sessionProfileMatches(home sessionmodel.ProfileHome, selector string) bool {
	selector = strings.TrimSpace(selector)
	return selector != "" && (strings.EqualFold(home.Name, selector) ||
		strings.EqualFold(home.AccountID, selector) ||
		strings.EqualFold(strings.TrimSpace(home.Email), selector))
}

func (service *Catalog) collectSharedReports(
	ctx context.Context,
	homes []sessionmodel.ProfileHome,
	query sessionmodel.Query,
) ([]sessionmodel.Report, error) {
	stored, err := service.reader.List(ctx, service.sharedCodexHome)
	if err != nil {
		return nil, err
	}
	reports := make([]sessionmodel.Report, 0, len(stored))
	for _, session := range stored {
		report, err := service.sharedSessionReport(ctx, homes, session)
		if err != nil {
			return nil, err
		}
		if query.Profile != "" {
			bound, ok := sessionProfileByName(homes, report.Profile)
			if !ok || !sessionProfileMatches(bound, query.Profile) {
				continue
			}
		}
		if matchesSessionQuery(report, query) {
			reports = append(reports, report)
		}
	}
	return reports, nil
}

func (service *Catalog) sharedSessionReport(
	ctx context.Context,
	homes []sessionmodel.ProfileHome,
	stored sessionentity.Session,
) (sessionmodel.Report, error) {
	report := sessionmodel.Report{
		ID: stored.ID, ThreadName: stored.ThreadName, Preview: stored.Preview,
		UpdatedAt: stored.UpdatedAt, UpdatedUnix: stored.UpdatedUnix, CWD: stored.CWD,
		CodexHome: service.sharedCodexHome, ModelProvider: stored.ModelProvider,
		LastModel: stored.LastModel, LastReasoningEffort: stored.LastReasoningEffort,
		Source: stored.Source, Path: stored.Path, ParentThreadID: stored.ParentThreadID,
	}
	if service.ownerLookup == nil || strings.TrimSpace(stored.ID) == "" {
		return report, nil
	}
	owner, err := service.ownerLookup(ctx, stored.ID)
	if err != nil {
		return sessionmodel.Report{}, err
	}
	report.UpstreamAccountID = owner
	if owner == "" {
		return report, nil
	}
	matched := -1
	for index, home := range homes {
		if profileOwnsRoutingID(home, owner) {
			if matched >= 0 {
				return sessionmodel.Report{}, fmt.Errorf("session routing owner maps to multiple profiles")
			}
			matched = index
		}
	}
	if matched < 0 {
		return report, nil
	}
	home := homes[matched]
	report.Profile = home.Name
	report.AccountID = home.AccountID
	report.CodexHome = home.CodexHome
	return report, nil
}

func profileOwnsRoutingID(home sessionmodel.ProfileHome, owner string) bool {
	for _, candidate := range home.RoutingIDs {
		if candidate == owner {
			return true
		}
	}
	return false
}

func sessionProfileByName(homes []sessionmodel.ProfileHome, name string) (sessionmodel.ProfileHome, bool) {
	for _, home := range homes {
		if home.Name == name {
			return home, true
		}
	}
	return sessionmodel.ProfileHome{}, false
}

func (service *Catalog) collectReports(ctx context.Context, homes []sessionmodel.ProfileHome, query sessionmodel.Query) ([]sessionmodel.Report, error) {
	reports := make([]sessionmodel.Report, 0)
	for _, home := range homes {
		if query.Profile != "" && !sessionProfileMatches(home, query.Profile) {
			continue
		}
		profileReports, err := service.reader.List(ctx, home.CodexHome)
		if err != nil {
			return nil, err
		}
		reports = append(reports, matchingReports(home, profileReports, query)...)
	}
	return reports, nil
}

func matchingReports(home sessionmodel.ProfileHome, stored []sessionentity.Session, query sessionmodel.Query) []sessionmodel.Report {
	reports := make([]sessionmodel.Report, 0, len(stored))
	for _, session := range stored {
		report := sessionReport(home, session)
		if matchesSessionQuery(report, query) {
			reports = append(reports, report)
		}
	}
	return reports
}

func sessionReport(home sessionmodel.ProfileHome, stored sessionentity.Session) sessionmodel.Report {
	return sessionmodel.Report{
		ID: stored.ID, ThreadName: stored.ThreadName, Preview: stored.Preview,
		UpdatedAt: stored.UpdatedAt, UpdatedUnix: stored.UpdatedUnix, CWD: stored.CWD,
		ModelProvider: stored.ModelProvider, LastModel: stored.LastModel, LastReasoningEffort: stored.LastReasoningEffort,
		Source: stored.Source, Path: stored.Path, ParentThreadID: stored.ParentThreadID,
		Profile: home.Name, AccountID: home.AccountID,
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
