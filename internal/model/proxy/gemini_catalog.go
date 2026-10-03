package proxy

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed gemini_provider_catalog_0_435_1.json
var geminiProviderCatalogSource []byte

func GeminiProviderCatalog() ([]ProviderCatalogEntry, error) {
	var entries []ProviderCatalogEntry
	if err := json.Unmarshal(geminiProviderCatalogSource, &entries); err != nil {
		return nil, errors.New("parse embedded Gemini provider catalog")
	}
	if len(entries) == 0 || len(entries) > ProviderCatalogMaxItems {
		return nil, errors.New("embedded Gemini provider catalog has invalid size")
	}
	return entries, nil
}

func GeminiModelsAPI() ([]map[string]any, error) {
	entries, err := GeminiProviderCatalog()
	if err != nil {
		return nil, err
	}
	return providerCatalogJSON(entries), nil
}
