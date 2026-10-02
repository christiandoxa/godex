package runtime

import (
	"fmt"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func buildAnthropicExternalCatalog(
	provider proxymodel.Provider,
	arguments []string,
) ([]map[string]any, error) {
	launchModel := effectiveProviderModel(provider, arguments)
	contextWindow, autoCompact, err := anthropicLaunchLimits(provider, arguments)
	if err != nil {
		return nil, err
	}
	entries, err := proxymodel.AnthropicProviderCatalog()
	if err != nil {
		return nil, err
	}
	return anthropicCatalogModels(launchModel, contextWindow, autoCompact, entries)
}

func anthropicLaunchLimits(provider proxymodel.Provider, arguments []string) (uint64, uint64, error) {
	contextWindow, err := effectiveProviderUint(arguments, "model_context_window", uint64(provider.ContextWindow))
	if err != nil {
		return 0, 0, err
	}
	autoCompact, err := effectiveProviderUint(arguments, "model_auto_compact_token_limit", uint64(provider.AutoCompactLimit))
	if err != nil {
		return 0, 0, err
	}
	if contextWindow > 0 && autoCompact >= contextWindow {
		autoCompact = contextWindow - 1
	}
	return contextWindow, autoCompact, nil
}

func anthropicCatalogModels(
	launchModel string,
	contextWindow, autoCompact uint64,
	entries []proxymodel.ProviderCatalogEntry,
) ([]map[string]any, error) {
	models := make([]map[string]any, 0, len(entries)+1)
	seen := make(map[string]bool, len(entries)+1)
	if err := appendAnthropicCatalogModel(
		&models, seen, launchModel,
		anthropicCatalogContext(launchModel, contextWindow, entries),
		autoCompact, entries,
	); err != nil {
		return nil, err
	}
	for _, entry := range entries {
		modelContext := contextWindow
		if entry.ContextWindowTokens != nil {
			modelContext = *entry.ContextWindowTokens
		}
		modelCompact := autoCompact
		if entry.ContextWindowTokens != nil {
			modelCompact = modelContext * 95 / 100
		}
		if err := appendAnthropicCatalogModel(
			&models, seen, entry.ID, modelContext, modelCompact, entries,
		); err != nil {
			return nil, err
		}
	}
	return models, nil
}

func appendAnthropicCatalogModel(
	models *[]map[string]any,
	seen map[string]bool,
	slug string,
	contextWindow, autoCompact uint64,
	entries []proxymodel.ProviderCatalogEntry,
) error {
	slug = strings.TrimSpace(slug)
	key := strings.ToLower(slug)
	if slug == "" || seen[key] {
		return nil
	}
	if len(*models) >= proxymodel.ProviderCatalogMaxItems {
		return fmt.Errorf("provider model catalog exceeds the hard limit of %d entries", proxymodel.ProviderCatalogMaxItems)
	}
	seen[key] = true
	displayName, description := anthropicCatalogMetadata(slug, entries)
	if contextWindow > 0 && autoCompact >= contextWindow {
		autoCompact = contextWindow - 1
	}
	*models = append(*models, externalCodexCatalogModel(
		slug, displayName, description, len(*models)+1,
		contextWindow, autoCompact, entries,
	))
	return nil
}

func anthropicCatalogContext(
	model string,
	fallback uint64,
	entries []proxymodel.ProviderCatalogEntry,
) uint64 {
	if entry := proxymodel.ResolveProviderCatalogEntry(entries, model); entry != nil && entry.ContextWindowTokens != nil {
		return *entry.ContextWindowTokens
	}
	return fallback
}

func anthropicCatalogMetadata(
	slug string,
	entries []proxymodel.ProviderCatalogEntry,
) (string, string) {
	if entry := proxymodel.ResolveProviderCatalogEntry(entries, slug); entry != nil {
		return entry.DisplayName, entry.Description
	}
	return strings.TrimSpace(slug), "External provider model routed through the Prodex Responses adapter."
}
