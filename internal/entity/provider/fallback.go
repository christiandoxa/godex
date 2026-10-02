package provider

import "strings"

const (
	modelClaudeSonnet46 = "claude-sonnet-4-6"
	modelClaudeOpus48   = "claude-opus-4-8"
	modelClaudeHaiku45  = "claude-haiku-4-5"
	modelGPT53Codex     = "gpt-5.3-codex"
	modelGPT51Codex     = "gpt-5.1-codex"
	modelGPT4o          = "gpt-4o"
)

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
	case "deepseek":
		return deepSeekFallbackChain(trimmed)
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
		return []string{modelClaudeSonnet46, modelClaudeOpus48, modelClaudeHaiku45}
	case "opus", "best":
		return []string{modelClaudeOpus48, modelClaudeSonnet46}
	case "sonnet", "pro":
		return []string{modelClaudeSonnet46, modelClaudeOpus48}
	case "haiku", "flash":
		return []string{modelClaudeHaiku45, modelClaudeSonnet46}
	default:
		return []string{model}
	}
}

func copilotFallbackChain(model string) []string {
	switch strings.ToLower(model) {
	case "", "auto", "default", "codex", "pro":
		return []string{modelGPT53Codex, modelGPT51Codex, modelGPT4o}
	case "gpt-5.5":
		return []string{"gpt-5.5", modelGPT53Codex, modelGPT51Codex, modelGPT4o}
	case "gpt-5.4":
		return []string{"gpt-5.4", modelGPT53Codex, modelGPT51Codex, modelGPT4o}
	case modelGPT53Codex:
		return []string{modelGPT53Codex, modelGPT51Codex, modelGPT4o}
	case "claude", "sonnet":
		return []string{modelClaudeSonnet46, modelGPT53Codex, modelGPT51Codex}
	case "gemini":
		return []string{"gemini-3.1-pro-preview", modelGPT53Codex, modelGPT51Codex}
	default:
		return []string{model}
	}
}

func deepSeekFallbackChain(model string) []string {
	switch strings.ToLower(model) {
	case "", "auto", "pro":
		return []string{"deepseek-v4-pro", "deepseek-v4-flash"}
	case "flash":
		return []string{"deepseek-v4-flash", "deepseek-v4-pro"}
	default:
		return []string{model}
	}
}
