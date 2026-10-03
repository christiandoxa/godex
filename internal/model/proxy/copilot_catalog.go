package proxy

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
)

const (
	CopilotRuntimeCatalogFile   = "prodex-copilot-runtime-model-catalog.json"
	ExternalProviderCatalogFile = "prodex-external-provider-model-catalog.json"
	DeepSeekModelCatalogFile    = "prodex-deepseek-model-catalog.json"
	KiroProfileModelCatalogFile = "kiro_model_catalog.json"
	ProviderCatalogMaxItems     = 1024
	ProviderCatalogMaxBytes     = 1 << 20
)

const (
	copilotDisplayNameKey = "display_name"
	copilotDescriptionKey = "description"
)

//go:embed copilot_provider_catalog_0_435_1.json
var copilotProviderCatalogSource []byte

type ProviderCatalogEntry struct {
	Provider                     string            `json:"provider"`
	OwnedBy                      string            `json:"owned_by"`
	ID                           string            `json:"id"`
	DisplayName                  string            `json:"display_name"`
	Description                  string            `json:"description"`
	ContextWindowTokens          *uint64           `json:"context_window_tokens"`
	MaxOutputTokens              *uint64           `json:"max_output_tokens"`
	DefaultOutputReserveTokens   *uint64           `json:"default_output_reserve_tokens"`
	SupportedReasoningEfforts    []string          `json:"supported_reasoning_efforts"`
	DefaultReasoningEffort       *string           `json:"default_reasoning_effort"`
	ReasoningReserveTokens       map[string]uint64 `json:"reasoning_reserve_tokens"`
	EmbeddingCompatible          *bool             `json:"embedding_compatible"`
	InputCostPerMillionMicrousd  *uint64           `json:"input_cost_per_million_microusd"`
	OutputCostPerMillionMicrousd *uint64           `json:"output_cost_per_million_microusd"`
	SupportedEndpoints           []string          `json:"supported_endpoints"`
	Aliases                      []string          `json:"aliases"`
	FeatureFlags                 map[string]any    `json:"feature_flags"`
	PricingKnown                 bool              `json:"pricing_known"`
}

func CopilotProviderCatalog() ([]ProviderCatalogEntry, error) {
	var entries []ProviderCatalogEntry
	if err := json.Unmarshal(copilotProviderCatalogSource, &entries); err != nil {
		return nil, errors.New("parse embedded Copilot provider catalog")
	}
	if len(entries) == 0 || len(entries) > ProviderCatalogMaxItems {
		return nil, errors.New("embedded Copilot provider catalog has invalid size")
	}
	return entries, nil
}

func ResolveProviderCatalogEntry(entries []ProviderCatalogEntry, model string) *ProviderCatalogEntry {
	model = strings.TrimSpace(model)
	for index := range entries {
		if strings.EqualFold(entries[index].ID, model) {
			return &entries[index]
		}
		for _, alias := range entries[index].Aliases {
			if strings.EqualFold(alias, model) {
				return &entries[index]
			}
		}
	}
	return nil
}

func CopilotProviderCatalogJSON() ([]map[string]any, error) {
	entries, err := CopilotProviderCatalog()
	if err != nil {
		return nil, err
	}
	return providerCatalogJSON(entries), nil
}

func providerCatalogJSON(entries []ProviderCatalogEntry) []map[string]any {
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		result = append(result, map[string]any{
			"id":                               entry.ID,
			"object":                           "model",
			"provider":                         entry.Provider,
			"owned_by":                         entry.OwnedBy,
			copilotDisplayNameKey:              entry.DisplayName,
			copilotDescriptionKey:              entry.Description,
			"context_window":                   entry.ContextWindowTokens,
			"max_output_tokens":                entry.MaxOutputTokens,
			"default_output_reserve_tokens":    entry.DefaultOutputReserveTokens,
			"supported_reasoning_efforts":      nilOrStrings(entry.SupportedReasoningEfforts),
			"default_reasoning_effort":         entry.DefaultReasoningEffort,
			"reasoning_reserve_tokens":         nilOrMap(entry.ReasoningReserveTokens),
			"embedding_compatible":             entry.EmbeddingCompatible,
			"input_cost_per_million_microusd":  entry.InputCostPerMillionMicrousd,
			"output_cost_per_million_microusd": entry.OutputCostPerMillionMicrousd,
			"endpoints":                        append([]string(nil), entry.SupportedEndpoints...),
			"aliases":                          append([]string{}, entry.Aliases...),
			"feature_flags":                    cloneAnyMap(entry.FeatureFlags),
			"pricing_known":                    entry.PricingKnown,
		})
	}
	return result
}

func nilOrStrings(values []string) any {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func nilOrMap(values map[string]uint64) any {
	if len(values) == 0 {
		return nil
	}
	copy := make(map[string]uint64, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	copy := make(map[string]any, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}
