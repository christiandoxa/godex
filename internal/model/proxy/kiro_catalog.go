package proxy

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed kiro_provider_catalog_0_435_1.json
var kiroProviderCatalogSource []byte

func KiroProviderCatalog() ([]ProviderCatalogEntry, error) {
	var entries []ProviderCatalogEntry
	if err := json.Unmarshal(kiroProviderCatalogSource, &entries); err != nil {
		return nil, errors.New("parse embedded Kiro provider catalog")
	}
	if len(entries) == 0 || len(entries) > ProviderCatalogMaxItems {
		return nil, errors.New("embedded Kiro provider catalog has invalid size")
	}
	return entries, nil
}

func KiroModelsAPI() ([]map[string]any, error) {
	entries, err := KiroProviderCatalog()
	if err != nil {
		return nil, err
	}
	return providerCatalogJSON(entries), nil
}
