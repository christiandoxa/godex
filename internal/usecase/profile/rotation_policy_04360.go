package profile

import (
	"context"
	"os"
)

// RuntimeRotationEligible matches the default non-forced proxy policy in
// Prodex 0.436.0 runtime_launch/profile.rs. A second profile only helps
// if the selected home exists and at least one profile is quota-compatible;
// an unauthenticated synthetic profile does not enable rotation.
func (catalog *Catalog) RuntimeRotationEligible(ctx context.Context, selectedName string) (bool, error) {
	if catalog == nil || ctx.Err() != nil {
		return false, ctx.Err()
	}
	reports, err := catalog.List(ctx)
	if err != nil {
		return false, err
	}
	if len(reports) <= 1 {
		return false, nil
	}
	selectedHome := ""
	for _, report := range reports {
		if report.Profile.Name == selectedName {
			selectedHome = report.Profile.CodexHome
			break
		}
	}
	if selectedHome == "" {
		return false, nil
	}
	stat, err := os.Stat(selectedHome)
	if err != nil || !stat.IsDir() {
		return false, nil
	}
	for _, report := range reports {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if !report.Enabled {
			continue
		}
		auth, err := catalog.quotaAuthSummary(ctx, report.Profile)
		if err == nil && auth.Compatible {
			return true, nil
		}
	}
	return false, nil
}
