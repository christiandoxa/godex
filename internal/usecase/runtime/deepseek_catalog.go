package runtime

import (
	"fmt"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	deepSeekCatalogDescriptionKey = "description"
	deepSeekCatalogEffortKey      = "effort"
)

const deepSeekBaseInstructions = "You are Codex, a coding agent. You and the user share the same workspace.\n\nFocus on the user's software task. Inspect the codebase before changing behavior, make narrow edits, preserve user changes, and verify with relevant tests or commands when feasible.\n\nUse tools deliberately. For shell work, prefer fast focused commands. For file edits, keep changes minimal and explain non-obvious logic in short comments only when useful."

type deepSeekCatalogSeed struct {
	slug, name, description string
}

var deepSeekCatalogSeeds = []deepSeekCatalogSeed{
	{"auto", "DeepSeek Auto", "Godex DeepSeek fallback chain routed through current DeepSeek models."},
	{"pro", "DeepSeek Pro", "Godex DeepSeek Pro alias routed through DeepSeek V4 Pro."},
	{"flash", "DeepSeek Flash", "Godex DeepSeek Flash alias routed through DeepSeek V4 Flash."},
	{"deepseek-v4-pro", "DeepSeek V4 Pro", "DeepSeek V4 Pro routed through the Godex Responses adapter."},
	{"deepseek-v4-flash", "DeepSeek V4 Flash", "DeepSeek V4 Flash routed through the Godex Responses adapter."},
	{"deepseek-chat", "DeepSeek Chat", "DeepSeek chat compatibility model routed through the Godex Responses adapter."},
	{"deepseek-reasoner", "DeepSeek Reasoner", "DeepSeek reasoner compatibility model routed through the Godex Responses adapter."},
}

func buildDeepSeekCodexCatalog(provider proxymodel.Provider, arguments []string) ([]map[string]any, error) {
	launchModel := effectiveProviderModel(provider, arguments)
	contextWindow, err := effectiveProviderUint(arguments, "model_context_window", uint64(provider.ContextWindow))
	if err != nil {
		return nil, err
	}
	autoCompact, err := effectiveProviderUint(arguments, "model_auto_compact_token_limit", uint64(provider.AutoCompactLimit))
	if err != nil {
		return nil, err
	}
	if contextWindow > 0 && autoCompact >= contextWindow {
		autoCompact = contextWindow - 1
	}
	candidates := make([]string, 0, len(deepSeekCatalogSeeds)+1)
	candidates = append(candidates, launchModel)
	for _, seed := range deepSeekCatalogSeeds {
		candidates = append(candidates, seed.slug)
	}
	seen := make(map[string]bool, len(candidates))
	models := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		slug := strings.TrimSpace(candidate)
		key := strings.ToLower(slug)
		if slug == "" || seen[key] {
			continue
		}
		if len(models) >= proxymodel.ProviderCatalogMaxItems {
			return nil, fmt.Errorf("provider model catalog exceeds the hard limit of %d entries", proxymodel.ProviderCatalogMaxItems)
		}
		seen[key] = true
		name, description := deepSeekCatalogMetadata(slug)
		models = append(models, deepSeekCodexCatalogModel(slug, name, description, len(models)+1, contextWindow, autoCompact))
	}
	return models, nil
}

func deepSeekCatalogMetadata(slug string) (string, string) {
	for _, seed := range deepSeekCatalogSeeds {
		if strings.EqualFold(seed.slug, slug) {
			return seed.name, seed.description
		}
	}
	return slug, "DeepSeek model routed through the Godex Responses adapter."
}

func deepSeekCodexCatalogModel(slug, displayName, description string, priority int, contextWindow, autoCompact uint64) map[string]any {
	levels := []any{
		map[string]any{deepSeekCatalogEffortKey: "low", deepSeekCatalogDescriptionKey: "DeepSeek low reasoning effort"},
		map[string]any{deepSeekCatalogEffortKey: "medium", deepSeekCatalogDescriptionKey: "DeepSeek medium reasoning effort"},
		map[string]any{deepSeekCatalogEffortKey: "high", deepSeekCatalogDescriptionKey: "DeepSeek high reasoning effort"},
		map[string]any{deepSeekCatalogEffortKey: "xhigh", deepSeekCatalogDescriptionKey: "DeepSeek max reasoning effort"},
	}
	return map[string]any{
		"slug": slug, "display_name": displayName, deepSeekCatalogDescriptionKey: description,
		"default_reasoning_level": "high", "supported_reasoning_levels": levels,
		"shell_type": "shell_command", "visibility": "list", "supported_in_api": true,
		"priority": priority, "additional_speed_tiers": []any{}, "service_tiers": []any{},
		"default_service_tier": nil, "availability_nux": nil, "upgrade": nil,
		"base_instructions":            deepSeekBaseInstructions,
		"supports_reasoning_summaries": false, "supports_reasoning_summary_parameter": false,
		"default_reasoning_summary": "none", "support_verbosity": false, "default_verbosity": nil,
		"apply_patch_tool_type": "freeform", "web_search_tool_type": "text",
		"truncation_policy":            map[string]any{"mode": "tokens", "limit": 10_000},
		"supports_parallel_tool_calls": true, "supports_image_detail_original": false,
		"context_window": contextWindow, "max_context_window": contextWindow,
		"auto_compact_token_limit": autoCompact, "effective_context_window_percent": 95,
		"experimental_supported_tools": []any{}, "input_modalities": []any{"text"},
		"supports_search_tool": false,
	}
}
