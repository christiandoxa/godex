package proxy

import "time"

type Provider struct {
	Kind                string
	Name                string
	Host                string
	Login               string
	APIURL              string
	DefaultModel        string
	ContextWindow       int64
	AutoCompactLimit    int64
	StrictTools         bool
	WebSearchMode       string
	BetaBaseURL         string
	SSELookaheadTimeout time.Duration
}

type ProviderProfile struct {
	Name     string
	Home     string
	Provider Provider
	Enabled  bool
}
