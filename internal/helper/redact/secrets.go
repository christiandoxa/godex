package redact

import "regexp"

var patterns = []struct {
	pattern *regexp.Regexp
	replace string
}{
	{regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s]+`), `${1}<redacted>`},
	{regexp.MustCompile(`(?i)([?&](?:access_token|refresh_token|id_token|api_key|openai_api_key|anthropic_api_key|gemini_api_key|google_api_key|github_copilot_api_key)=)[^&#\s]+`), `${1}<redacted>`},
	{regexp.MustCompile(`(?i)(["']?(?:access_token|refresh_token|id_token|api_key|openai_api_key|anthropic_api_key|gemini_api_key|google_api_key|github_copilot_api_key)["']?\s*[:=]\s*["']?)[^"',\s}]+`), `${1}<redacted>`},
	{regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`), `${1}<redacted>@`},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`), `<redacted>`},
	{regexp.MustCompile(`\bgh[opsu]_[A-Za-z0-9]{20,}`), `<redacted>`},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), `<redacted>`},
}

// Secrets redacts common credential material while preserving surrounding diagnostics.
func Secrets(value string) string {
	for _, rule := range patterns {
		value = rule.pattern.ReplaceAllString(value, rule.replace)
	}
	return value
}
