package proxy

import (
	_ "embed"
	"encoding/json"
	"errors"
)

type AnthropicExternalCatalogSeed struct {
	Slug        string
	DisplayName string
	Description string
}

//go:embed anthropic_provider_catalog_0_434_3.json
var anthropicProviderCatalogSource []byte

func AnthropicProviderCatalog() ([]ProviderCatalogEntry, error) {
	var entries []ProviderCatalogEntry
	if err := json.Unmarshal(anthropicProviderCatalogSource, &entries); err != nil {
		return nil, errors.New("parse embedded Anthropic provider catalog")
	}
	if len(entries) == 0 || len(entries) > ProviderCatalogMaxItems {
		return nil, errors.New("embedded Anthropic provider catalog has invalid size")
	}
	return entries, nil
}

func AnthropicModelsAPI() ([]map[string]any, error) {
	entries, err := AnthropicProviderCatalog()
	if err != nil {
		return nil, err
	}
	return providerCatalogJSON(entries), nil
}

func AnthropicExternalCatalogSeeds() []AnthropicExternalCatalogSeed {
	return []AnthropicExternalCatalogSeed{
		{"auto", "Claude Auto", "Anthropic auto model routed through the Prodex Responses adapter."},
		{"opus", "Claude Opus", "Claude Opus alias routed through the Prodex Responses adapter."},
		{"sonnet", "Claude Sonnet", "Claude Sonnet alias routed through the Prodex Responses adapter."},
		{"haiku", "Claude Haiku", "Claude Haiku alias routed through the Prodex Responses adapter."},
		{"claude-opus-4-8", "Claude Opus 4.8", "Claude Opus 4.8 routed through the Prodex Responses adapter."},
		{"claude-sonnet-4-6", "Claude Sonnet 4.6", "Claude Sonnet 4.6 routed through the Prodex Responses adapter."},
		{"claude-haiku-4-5", "Claude Haiku 4.5", "Claude Haiku 4.5 routed through the Prodex Responses adapter."},
		{"claude-opus-4-6", "Claude Opus 4.6", "Claude Opus 4.6 routed through the Prodex Responses adapter."},
		{"claude-opus-4-20250514", "Claude Opus 4", "Claude Opus 4 routed through the Prodex Responses adapter."},
	}
}
