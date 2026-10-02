package proxy

type Provider struct {
	Kind             string
	Name             string
	Host             string
	Login            string
	APIURL           string
	DefaultModel     string
	ContextWindow    int64
	AutoCompactLimit int64
	StrictTools      bool
	WebSearchMode    string
	BetaBaseURL      string
}

type ProviderProfile struct {
	Name     string
	Home     string
	Provider Provider
	Enabled  bool
}
