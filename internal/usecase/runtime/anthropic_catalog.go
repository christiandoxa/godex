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
	seeds := proxymodel.AnthropicExternalCatalogSeeds()
	return anthropicCatalogModels(launchModel, contextWindow, autoCompact, seeds)
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
	seeds []proxymodel.AnthropicExternalCatalogSeed,
) ([]map[string]any, error) {
	models := make([]map[string]any, 0, len(seeds)+1)
	seen := make(map[string]bool, len(seeds)+1)
	displayName, description := anthropicSeedMetadata(launchModel, seeds)
	if err := appendAnthropicCatalogModel(&models, seen, launchModel, displayName, description, contextWindow, autoCompact); err != nil {
		return nil, err
	}
	for _, seed := range seeds {
		if err := appendAnthropicCatalogModel(&models, seen, seed.Slug, seed.DisplayName, seed.Description, contextWindow, autoCompact); err != nil {
			return nil, err
		}
	}
	return models, nil
}

func appendAnthropicCatalogModel(
	models *[]map[string]any,
	seen map[string]bool,
	slug, displayName, description string,
	contextWindow, autoCompact uint64,
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
	if displayName == "" {
		displayName = slug
	}
	if description == "" {
		description = "External provider model routed through the Prodex Responses adapter."
	}
	*models = append(*models, externalCodexCatalogModel(
		slug, displayName, description, len(*models)+1,
		contextWindow, autoCompact, nil,
	))
	return nil
}

func anthropicSeedMetadata(
	slug string,
	seeds []proxymodel.AnthropicExternalCatalogSeed,
) (string, string) {
	for _, seed := range seeds {
		if strings.EqualFold(seed.Slug, strings.TrimSpace(slug)) {
			return seed.DisplayName, seed.Description
		}
	}
	return strings.TrimSpace(slug), ""
}
