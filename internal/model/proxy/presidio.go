package proxy

import "time"

type PresidioConfig struct {
	AnalyzerURL      string
	AnonymizerURL    string
	Languages        []string
	LanguageMode     string
	FailClosed       bool
	TrustedHosts     []string
	Timeout          time.Duration
	MaxResponseBytes int
	MaxConcurrency   int
}
