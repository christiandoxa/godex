package runtime

import (
	"context"
	"strings"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

type recoveryProfileSource04360 interface {
	SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error)
}

// canonicalRecoveryAccountID04360 resolves command selectors before they are
// used to exclude the current profile. If the profile source cannot resolve a
// selector, recovery fails closed.
func canonicalRecoveryAccountID04360(
	ctx context.Context, profiles any, selector string,
) (string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", true
	}
	source, ok := profiles.(recoveryProfileSource04360)
	if !ok {
		return selector, true
	}
	homes, err := source.SessionProfiles(ctx)
	if err != nil {
		return "", false
	}
	match := ""
	for _, home := range homes {
		if home.AccountID == "" || !recoveryProfileMatchesSelector04360(home, selector) {
			continue
		}
		if match != "" && match != home.AccountID {
			return "", false
		}
		match = home.AccountID
	}
	if match == "" {
		return "", false
	}
	return match, true
}

func recoveryProfileMatchesSelector04360(home sessionmodel.ProfileHome, selector string) bool {
	selector = strings.TrimSpace(selector)
	return strings.EqualFold(strings.TrimSpace(home.Name), selector) ||
		strings.EqualFold(strings.TrimSpace(home.AccountID), selector) ||
		strings.EqualFold(strings.TrimSpace(home.Email), selector)
}

func recoveryExcludedAccountIDs04360(
	ctx context.Context, profiles any, report sessionmodel.Report,
) (map[string]bool, bool) {
	excluded := make(map[string]bool, 2)
	for index, selector := range []string{report.AccountID, report.UpstreamAccountID} {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		accountID, ok := canonicalRecoveryAccountID04360(ctx, profiles, selector)
		if !ok {
			if index == 0 {
				return nil, false
			}
			excluded[selector] = true
			continue
		}
		excluded[accountID] = true
	}
	return excluded, true
}
