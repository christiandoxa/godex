package proxy

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed anthropic_provider_catalog_0_435_1.json
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
