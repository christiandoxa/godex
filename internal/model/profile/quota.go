package profile

type QuotaAuthSummary struct {
	Label      string
	Compatible bool
}

type ModelProviderSetting struct {
	ProviderID string
	Source     string
}

type QuotaTarget struct {
	Name           string
	CodexHome      string
	Email          string
	Provider       string
	ProviderConfig ProviderSnapshot
	Auth           string
	AccountID      string
	Active         bool
	Enabled        bool
	Compatible     bool
}
