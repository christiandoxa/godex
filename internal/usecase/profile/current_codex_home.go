package profile

import "context"

func (catalog *Catalog) CurrentCodexHome(ctx context.Context) (string, error) {
	reports, err := catalog.List(ctx)
	if err != nil {
		return "", err
	}
	for _, report := range reports {
		if report.Active {
			return report.Profile.CodexHome, nil
		}
	}
	return catalog.currentCodexHome, nil
}
