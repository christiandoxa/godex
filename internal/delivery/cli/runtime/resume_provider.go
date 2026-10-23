package runtime

import (
	"errors"
	"strings"
)

type resumeProviderPlan struct {
	kind   string
	direct bool
}

func resumeProviderIdentity(value string) (resumeProviderPlan, error) {
	provider := []byte(strings.TrimSpace(value))
	// Registry identities fold ASCII only; Unicode lookalikes remain unknown.
	for index, char := range provider {
		if char >= 'A' && char <= 'Z' {
			provider[index] += 'a' - 'A'
		}
	}
	// Existing Godex rollouts retain their Godex-written provider IDs.
	switch string(provider) {
	case "", "openai", "openai-responses", "openai_compatible", "openai-compatible",
		"godex-openai", "godex-openai-governed-http", "prodex-openai-governed-http",
		"godex-openai-compatible", "godex-local", "prodex-local", "local", "local-openai", "local_openai":
		return resumeProviderPlan{}, nil
	case "amazon-bedrock", "amazon-bedrock-runtime":
		return resumeProviderPlan{direct: true}, nil
	case "anthropic", "claude", "godex-anthropic", "prodex-anthropic":
		return resumeProviderPlan{kind: anthropicProviderKind}, nil
	case "copilot", "github-copilot", "github_copilot", "godex-copilot", "prodex-copilot":
		return resumeProviderPlan{kind: copilotProviderKind}, nil
	case "deepseek", "godex-deepseek", "prodex-deepseek":
		return resumeProviderPlan{kind: deepSeekProviderKind}, nil
	case "gemini", "google", "godex-gemini", "prodex-gemini":
		return resumeProviderPlan{kind: geminiProviderKind}, nil
	case "kiro", "godex-kiro", "prodex-kiro":
		return resumeProviderPlan{kind: kiroProviderKind}, nil
	default:
		return resumeProviderPlan{}, errors.New(
			"resumed session has an unsupported provider identity; configure the matching provider or start a fresh session",
		)
	}
}
