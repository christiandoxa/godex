package runtime

import (
	"fmt"
	"strconv"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const catalogDescriptionKey = "description"

type providerCatalogStore interface {
	ReadCopilotRuntime(string) ([]map[string]any, error)
	WriteExternal(string, []map[string]any) (string, error)
}

type dynamicCopilotModel struct {
	slug          string
	displayName   string
	description   string
	contextWindow uint64
}

type catalogCandidate struct {
	slug          string
	contextWindow uint64
}

func prepareProviderRuntimeArguments(
	store providerCatalogStore,
	home string,
	provider proxymodel.Provider,
	arguments []string,
) ([]string, error) {
	if strings.TrimSpace(provider.Kind) == "" {
		return arguments, nil
	}
	defaults := providerDefaultArguments(provider)
	if store == nil || func() bool { _, found := providerConfigValue(arguments, "model_catalog_json"); return found }() {
		return append(defaults, arguments...), nil
	}
	var models []map[string]any
	var err error
	switch provider.Kind {
	case "copilot":
		models, err = buildCopilotExternalCatalog(store, home, provider, arguments)
	case "anthropic":
		models, err = buildAnthropicExternalCatalog(provider, arguments)
	default:
		return append(defaults, arguments...), nil
	}
	if err != nil {
		return nil, err
	}
	path, err := store.WriteExternal(home, models)
	if err != nil {
		return nil, err
	}
	managed := append([]string{"-c", "model_catalog_json=" + strconv.Quote(path)}, defaults...)
	return append(managed, arguments...), nil
}

func providerDefaultArguments(provider proxymodel.Provider) []string {
	defaults := make([]string, 0, 6)
	if provider.DefaultModel != "" {
		defaults = append(defaults, "-c", "model="+strconv.Quote(provider.DefaultModel))
	}
	if provider.ContextWindow > 0 {
		defaults = append(defaults, "-c", "model_context_window="+strconv.FormatInt(provider.ContextWindow, 10))
	}
	if provider.AutoCompactLimit > 0 {
		defaults = append(defaults, "-c", "model_auto_compact_token_limit="+strconv.FormatInt(provider.AutoCompactLimit, 10))
	}
	return defaults
}

func buildCopilotExternalCatalog(
	store providerCatalogStore,
	home string,
	provider proxymodel.Provider,
	arguments []string,
) ([]map[string]any, error) {
	dynamic, err := store.ReadCopilotRuntime(home)
	if err != nil {
		return nil, err
	}
	dynamicModels := parseDynamicCopilotModels(dynamic)
	seeds, err := proxymodel.CopilotExternalCatalogSeeds()
	if err != nil {
		return nil, err
	}
	providerEntries, err := proxymodel.CopilotProviderCatalog()
	if err != nil {
		return nil, err
	}
	launchModel := effectiveProviderModel(provider, arguments)
	contextWindow, err := effectiveProviderUint(arguments, "model_context_window", uint64(provider.ContextWindow))
	if err != nil {
		return nil, err
	}
	autoCompact, err := effectiveProviderUint(arguments, "model_auto_compact_token_limit", uint64(provider.AutoCompactLimit))
	if err != nil {
		return nil, err
	}
	candidates := copilotCatalogCandidates(launchModel, dynamicModels, seeds)
	models := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		if len(models) >= proxymodel.ProviderCatalogMaxItems {
			return nil, fmt.Errorf("provider model catalog exceeds the hard limit of %d entries", proxymodel.ProviderCatalogMaxItems)
		}
		dynamicModel := exactDynamicModel(dynamicModels, candidate.slug)
		displayName, description := copilotCatalogMetadata(candidate.slug, dynamicModel, seeds)
		modelContext := candidate.contextWindow
		if modelContext == 0 {
			modelContext = contextWindow
		}
		modelCompact := autoCompact
		if candidate.contextWindow > 0 {
			modelCompact = candidate.contextWindow * 95 / 100
		}
		if modelContext > 0 && modelCompact >= modelContext {
			modelCompact = modelContext - 1
		}
		models = append(models, externalCodexCatalogModel(
			candidate.slug, displayName, description, len(models)+1,
			modelContext, modelCompact, providerEntries,
		))
	}
	return models, nil
}

func copilotCatalogCandidates(
	launchModel string,
	dynamic []dynamicCopilotModel,
	seeds []proxymodel.CopilotExternalCatalogSeed,
) []catalogCandidate {
	result := make([]catalogCandidate, 0, 1+len(dynamic)+len(seeds))
	seen := make(map[string]bool, cap(result))
	appendCandidate := func(slug string, contextWindow uint64) {
		slug = strings.TrimSpace(slug)
		if slug == "" || seen[strings.ToLower(slug)] {
			return
		}
		seen[strings.ToLower(slug)] = true
		result = append(result, catalogCandidate{slug: slug, contextWindow: contextWindow})
	}
	launchDynamic := exactDynamicModel(dynamic, launchModel)
	launchContext := copilotModelPromptLimit(launchModel)
	if launchDynamic != nil && launchDynamic.contextWindow > 0 {
		launchContext = launchDynamic.contextWindow
	}
	appendCandidate(launchModel, launchContext)
	for _, model := range dynamic {
		contextWindow := model.contextWindow
		if contextWindow == 0 {
			contextWindow = copilotModelPromptLimit(model.slug)
		}
		appendCandidate(model.slug, contextWindow)
	}
	for _, seed := range seeds {
		appendCandidate(seed.Slug, copilotModelPromptLimit(seed.Slug))
	}
	return result
}

func parseDynamicCopilotModels(values []map[string]any) []dynamicCopilotModel {
	result := make([]dynamicCopilotModel, 0, len(values))
	for _, value := range values {
		slug := catalogMapString(value, "id", "slug", "model")
		if slug == "" {
			continue
		}
		contextWindow := catalogMapUint(value, "max_prompt_tokens")
		if contextWindow == 0 {
			if capabilities, ok := value["capabilities"].(map[string]any); ok {
				if limits, ok := capabilities["limits"].(map[string]any); ok {
					contextWindow = catalogMapUint(limits, "max_prompt_tokens")
				}
			}
		}
		if contextWindow == 0 {
			contextWindow = catalogMapUint(value, "context_window_tokens", "context_window")
		}
		result = append(result, dynamicCopilotModel{
			slug:          slug,
			displayName:   catalogMapString(value, "name", "model_name"),
			description:   catalogMapString(value, catalogDescriptionKey),
			contextWindow: contextWindow,
		})
	}
	return result
}

func exactDynamicModel(models []dynamicCopilotModel, slug string) *dynamicCopilotModel {
	for index := range models {
		if strings.EqualFold(models[index].slug, slug) {
			return &models[index]
		}
	}
	return nil
}

func copilotCatalogMetadata(
	slug string,
	dynamic *dynamicCopilotModel,
	seeds []proxymodel.CopilotExternalCatalogSeed,
) (string, string) {
	fallbackName, fallbackDescription := slug, "External provider model routed through the Prodex Responses adapter."
	for _, seed := range seeds {
		if strings.EqualFold(seed.Slug, slug) {
			fallbackName, fallbackDescription = seed.DisplayName, seed.Description
			break
		}
	}
	if dynamic == nil {
		return fallbackName, fallbackDescription
	}
	displayName, description := dynamic.displayName, dynamic.description
	if displayName == "" {
		displayName = fallbackName
	}
	if description == "" {
		description = fallbackDescription
	}
	return displayName, description
}

func externalCodexCatalogModel(
	slug, displayName, description string,
	priority int,
	contextWindow, autoCompact uint64,
	providerEntries []proxymodel.ProviderCatalogEntry,
) map[string]any {
	efforts, defaultEffort := copilotReasoningLevels(slug, providerEntries)
	return map[string]any{
		"slug":                                 slug,
		"display_name":                         displayName,
		catalogDescriptionKey:                  description,
		"default_reasoning_level":              defaultEffort,
		"supported_reasoning_levels":           efforts,
		"shell_type":                           "shell_command",
		"visibility":                           "list",
		"supported_in_api":                     true,
		"priority":                             priority,
		"additional_speed_tiers":               []any{},
		"service_tiers":                        []any{},
		"default_service_tier":                 nil,
		"availability_nux":                     nil,
		"upgrade":                              nil,
		"base_instructions":                    "",
		"supports_reasoning_summaries":         true,
		"supports_reasoning_summary_parameter": true,
		"default_reasoning_summary":            "none",
		"support_verbosity":                    false,
		"default_verbosity":                    nil,
		"apply_patch_tool_type":                "freeform",
		"web_search_tool_type":                 "text",
		"truncation_policy":                    map[string]any{"mode": "tokens", "limit": 10_000},
		"supports_parallel_tool_calls":         true,
		"supports_image_detail_original":       false,
		"context_window":                       contextWindow,
		"max_context_window":                   contextWindow,
		"auto_compact_token_limit":             autoCompact,
		"effective_context_window_percent":     95,
		"experimental_supported_tools":         []any{},
		"input_modalities":                     []string{"text", "image"},
		"supports_search_tool":                 true,
	}
}

func copilotReasoningLevels(
	slug string,
	entries []proxymodel.ProviderCatalogEntry,
) ([]map[string]any, string) {
	labels := []string{"low", "medium", "high", "xhigh"}
	defaultEffort := "high"
	if entry := copilotProviderCatalogEntry(entries, slug); entry != nil {
		if len(entry.SupportedReasoningEfforts) > 0 {
			labels = append([]string(nil), entry.SupportedReasoningEfforts...)
		}
		if entry.DefaultReasoningEffort != nil && *entry.DefaultReasoningEffort != "" {
			defaultEffort = *entry.DefaultReasoningEffort
		}
	}
	levels := make([]map[string]any, 0, len(labels))
	for _, effort := range labels {
		if description := reasoningEffortDescription(effort); description != "" {
			levels = append(levels, map[string]any{"effort": effort, catalogDescriptionKey: description})
		}
	}
	return levels, defaultEffort
}

func copilotProviderCatalogEntry(
	entries []proxymodel.ProviderCatalogEntry,
	slug string,
) *proxymodel.ProviderCatalogEntry {
	for index := range entries {
		if strings.EqualFold(entries[index].ID, slug) {
			return &entries[index]
		}
		for _, alias := range entries[index].Aliases {
			if strings.EqualFold(alias, slug) {
				return &entries[index]
			}
		}
	}
	return nil
}

func reasoningEffortDescription(effort string) string {
	switch effort {
	case "none":
		return "No reasoning effort"
	case "minimal":
		return "Minimal reasoning effort"
	case "low":
		return "Low reasoning effort"
	case "medium":
		return "Medium reasoning effort"
	case "high":
		return "High reasoning effort"
	case "xhigh":
		return "Extra-high reasoning effort"
	case "max":
		return "Max reasoning effort"
	case "ultra":
		return "Ultra reasoning effort"
	default:
		return ""
	}
}

func copilotModelPromptLimit(model string) uint64 {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "auto", "codex", "gpt-5.3-codex", "gpt-5.1-codex", "gpt-5.1-codex-max", "gpt-5.1-codex-mini":
		return 272_000
	case "gpt-5.5", "gpt-5.4":
		return 922_000
	case "claude-sonnet-4.6", "claude-opus-4.8", "claude-opus-4.7", "claude-opus-4.6", "gemini-3.1-pro-preview", "gemini-3.5-flash":
		return 936_000
	case "gpt-5-mini", "gpt-5.4-mini", "gpt-5.4-nano", "raptor-mini":
		return 128_000
	default:
		return 0
	}
}

func catalogMapString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok {
			if text = strings.TrimSpace(text); text != "" {
				return text
			}
		}
	}
	return ""
}

func catalogMapUint(value map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		switch current := value[key].(type) {
		case float64:
			if current > 1 && current == float64(uint64(current)) {
				return uint64(current)
			}
		case uint64:
			if current > 1 {
				return current
			}
		case int:
			if current > 1 {
				return uint64(current)
			}
		}
	}
	return 0
}
