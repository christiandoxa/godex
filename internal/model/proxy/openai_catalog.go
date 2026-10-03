package proxy

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed openai_provider_catalog_0_435_1.json
var openAIProviderCatalogSource []byte

func OpenAIProviderCatalog() ([]ProviderCatalogEntry, error) {
	var entries []ProviderCatalogEntry
	if err := json.Unmarshal(openAIProviderCatalogSource, &entries); err != nil {
		return nil, errors.New("parse embedded OpenAI provider catalog")
	}
	if len(entries) == 0 || len(entries) > ProviderCatalogMaxItems {
		return nil, errors.New("embedded OpenAI provider catalog has invalid size")
	}
	return entries, nil
}
