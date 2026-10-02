package provider

import "strings"

const (
	modelClaudeSonnet55 = "claude-sonnet-5-5"
	modelClaudeOpus55   = "claude-opus-5-5"
	modelClaudeHaiku45  = "claude-haiku-4-5"
	modelGPT6Astra      = "gpt-6-astra"
	modelGPT61Sol       = "gpt-6.1-sol"
	modelGPT6Luna       = "gpt-6-luna"
	modelGPT6Sol        = "gpt-6-sol"
	modelGPT53Codex     = "gpt-5.3-codex"
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
		return []string{modelClaudeSonnet55, modelClaudeOpus55, modelClaudeHaiku45}
	case "opus", "best":
		return []string{modelClaudeOpus55, modelClaudeSonnet55}
	case "sonnet", "pro":
		return []string{modelClaudeSonnet55, modelClaudeOpus55}
	case "haiku", "flash":
		return []string{modelClaudeHaiku45, modelClaudeSonnet55}
	default:
		return []string{model}
	}
}

func copilotFallbackChain(model string) []string {
	switch strings.ToLower(model) {
	case "", "auto", "default", "codex", "pro":
		return []string{modelGPT6Astra, modelGPT61Sol, modelGPT53Codex}
	case "astra":
		return []string{modelGPT6Astra, modelGPT61Sol, modelGPT6Luna}
	case "sol":
		return []string{modelGPT61Sol, modelGPT6Sol, modelGPT6Luna}
	case "luna":
		return []string{modelGPT6Luna, modelGPT61Sol}
	case "gpt-5.5":
		return []string{"gpt-5.5", modelGPT61Sol, modelGPT53Codex}
	case "gpt-5.4":
		return []string{"gpt-5.4", modelGPT61Sol, modelGPT53Codex}
	case modelGPT53Codex:
		return []string{modelGPT53Codex, modelGPT61Sol, modelGPT6Luna}
	case "claude", "sonnet":
		return []string{modelClaudeSonnet55, modelGPT61Sol, modelGPT53Codex}
	case "gemini":
		return []string{"gemini-3.8-flash", modelGPT61Sol, modelGPT53Codex}
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
