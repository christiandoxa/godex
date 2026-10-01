package profile

type QuotaAuthSummary struct {
	Label      string
	Compatible bool
}

type QuotaTarget struct {
	Name       string
	CodexHome  string
	Email      string
	Provider   string
	Auth       string
	AccountID  string
	Active     bool
	Enabled    bool
	Compatible bool
}
