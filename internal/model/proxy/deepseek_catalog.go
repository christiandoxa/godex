package proxy

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed deepseek_provider_catalog_0_434_3.json
var deepSeekProviderCatalogSource []byte

func DeepSeekProviderCatalog() ([]ProviderCatalogEntry, error) {
	var entries []ProviderCatalogEntry
	if err := json.Unmarshal(deepSeekProviderCatalogSource, &entries); err != nil {
		return nil, errors.New("parse embedded DeepSeek provider catalog")
	}
	if len(entries) == 0 || len(entries) > ProviderCatalogMaxItems {
		return nil, errors.New("embedded DeepSeek provider catalog has invalid size")
	}
	return entries, nil
}

func DeepSeekModelsAPI() ([]map[string]any, error) {
	entries, err := DeepSeekProviderCatalog()
	if err != nil {
		return nil, err
	}
	return providerCatalogJSON(entries), nil
}
