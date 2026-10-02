package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (source *Source) runtimeCatalog(home string) ([]map[string]any, error) {
	static, err := proxymodel.KiroModelsAPI()
	if err != nil {
		return nil, err
	}
	text, found, err := readManagedQuotaFile(home, ModelCatalogFile)
	if err != nil || !found {
		return static, err
	}
	if err := source.ValidateModelCatalog(context.Background(), text); err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil, errors.New("failed to parse Kiro runtime model catalog")
	}
	models, ok := findModels(value)
	if !ok {
		return nil, errors.New("Kiro runtime model catalog is missing models")
	}
	dynamic := normalizeModels(models)
	return mergeKiroCatalog(static, dynamic)
}

func mergeKiroCatalog(static, dynamic []map[string]any) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(static)+len(dynamic))
	seen := make(map[string]bool, len(static)+len(dynamic))
	for _, set := range [][]map[string]any{static, dynamic} {
		for _, model := range set {
			id, _ := model["id"].(string)
			id = strings.TrimSpace(id)
			key := strings.ToLower(id)
			if id == "" || seen[key] {
				continue
			}
			if len(result) >= proxymodel.ProviderCatalogMaxItems {
				return nil, fmt.Errorf("Kiro runtime model catalog exceeds hard limit (%d)", proxymodel.ProviderCatalogMaxItems)
			}
			seen[key] = true
			copy := make(map[string]any, len(model))
			for name, value := range model {
				copy[name] = value
			}
			result = append(result, copy)
		}
	}
	return result, nil
}
