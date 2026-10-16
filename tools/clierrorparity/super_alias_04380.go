package main

import "strings"

// Parser-independent semantic projection of the selected provider in a
// native Super dry-run. Prodex uses prodex-<provider> in its internal model
// config, whereas Godex prints the public provider name. Require exact
// full-line labels; a quoted Codex tail or an arbitrary log line cannot
// masquerade as successful alias selection.
func cliDryRunProvider04380(output string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Provider: ") {
			continue
		}
		provider := strings.TrimSpace(strings.TrimPrefix(trimmed, "Provider: "))
		if provider == "" || strings.ContainsAny(provider, " \t\r\n") {
			return "", false
		}
		return provider, true
	}
	return "", false
}
