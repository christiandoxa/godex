package session

import (
	"context"
	"fmt"
	"strings"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

type missingSessionError struct{ selector string }

func (err missingSessionError) Error() string {
	return fmt.Sprintf("session %q was not found", err.selector)
}

func (catalog *Catalog) resolveProfileSession(ctx context.Context, profile, selector string) (sessionmodel.Report, error) {
	homes, err := catalog.sessionProfileHomes(ctx)
	if err != nil {
		return sessionmodel.Report{}, err
	}
	if err := validateProfileSelector(homes, profile); err != nil {
		return sessionmodel.Report{}, err
	}
	reports, err := catalog.collectReports(ctx, homes, sessionmodel.Query{Profile: profile})
	if err != nil {
		return sessionmodel.Report{}, err
	}
	return catalog.resolveReport(ctx, strings.TrimSpace(selector), reports)
}
