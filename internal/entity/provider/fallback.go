package provider

import "strings"

func ModelFallbackChain(providerKind, model string) []string {
	trimmed := strings.TrimSpace(model)
	if combo, ok := parseComboFallback(trimmed); ok {
		return combo
	}
	switch strings.ToLower(strings.TrimSpace(providerKind)) {
	case "anthropic":
		return anthropicFallbackChain(trimmed)
	case "copilot":
		return copilotFallbackChain(trimmed)
	default:
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}
}

func parseComboFallback(model string) ([]string, bool) {
	if !strings.HasPrefix(model, "combo:") {
		return nil, false
	}
	components := strings.FieldsFunc(strings.TrimPrefix(model, "combo:"), func(value rune) bool {
		return value == ',' || value == ';' || value == '|' || value == '>'
	})
	result := make([]string, 0, len(components))
	seen := make(map[string]bool, len(components))
	for _, component := range components {
		component = strings.TrimSpace(component)
		if component == "" {
			continue
		}
		key := strings.ToLower(component)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, component)
	}
	if len(result) == 0 {
		return nil, false
	}
	return result, true
}

func anthropicFallbackChain(model string) []string {
	switch strings.ToLower(model) {
	case "", "auto", "default":
		return []string{"claude-sonnet-4-6", "claude-opus-4-8", "claude-haiku-4-5"}
	case "opus", "best":
		return []string{"claude-opus-4-8", "claude-sonnet-4-6"}
	case "sonnet", "pro":
		return []string{"claude-sonnet-4-6", "claude-opus-4-8"}
	case "haiku", "flash":
		return []string{"claude-haiku-4-5", "claude-sonnet-4-6"}
	default:
		return []string{model}
	}
}

func copilotFallbackChain(model string) []string {
	switch strings.ToLower(model) {
	case "", "auto", "default", "codex", "pro":
		return []string{"gpt-5.3-codex", "gpt-5.1-codex", "gpt-4o"}
	case "gpt-5.5":
		return []string{"gpt-5.5", "gpt-5.3-codex", "gpt-5.1-codex", "gpt-4o"}
	case "gpt-5.4":
		return []string{"gpt-5.4", "gpt-5.3-codex", "gpt-5.1-codex", "gpt-4o"}
	case "gpt-5.3-codex":
		return []string{"gpt-5.3-codex", "gpt-5.1-codex", "gpt-4o"}
	case "claude", "sonnet":
		return []string{"claude-sonnet-4-6", "gpt-5.3-codex", "gpt-5.1-codex"}
	case "gemini":
		return []string{"gemini-3.1-pro-preview", "gpt-5.3-codex", "gpt-5.1-codex"}
	default:
		return []string{model}
	}
}
