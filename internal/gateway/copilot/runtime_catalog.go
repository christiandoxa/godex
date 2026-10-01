package copilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	modelCatalogMaxItems       = proxymodel.ProviderCatalogMaxItems
	runtimeCatalogModelKey     = "model"
	runtimeCatalogCapabilities = "capabilities"
	runtimeCatalogMaxPromptKey = "max_prompt_tokens"
)

func runtimeModelCatalog(body []byte) ([]map[string]any, error) {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, errors.New("invalid Copilot model catalog JSON")
	}
	raw := make([]map[string]any, 0)
	if collectRuntimeModels(value, &raw) {
		return nil, fmt.Errorf("Copilot model catalog exceeds the hard limit of %d entries", modelCatalogMaxItems)
	}
	dynamic := make([]map[string]any, 0, len(raw))
	for _, model := range raw {
		if entry := normalizeRuntimeCatalogEntry(model); entry != nil {
			dynamic = append(dynamic, entry)
		}
	}
	static, err := proxymodel.CopilotProviderCatalogJSON()
	if err != nil {
		return nil, err
	}
	return mergeRuntimeCatalog(static, dynamic)
}

func collectRuntimeModels(value any, output *[]map[string]any) bool {
	switch current := value.(type) {
	case map[string]any:
		return collectRuntimeModelMap(current, output)
	case []any:
		return collectRuntimeModelArray(current, output)
	default:
		return false
	}
}

func collectRuntimeModelMap(object map[string]any, output *[]map[string]any) bool {
	for key, nested := range object {
		if runtimeModelListKey(key) {
			if models, ok := nested.([]any); ok {
				if appendRuntimeModelList(models, output) {
					return true
				}
				continue
			}
		}
		if collectRuntimeModels(nested, output) {
			return true
		}
	}
	return false
}

func appendRuntimeModelList(models []any, output *[]map[string]any) bool {
	for _, model := range models {
		if len(*output) >= modelCatalogMaxItems {
			return true
		}
		if object, ok := model.(map[string]any); ok {
			*output = append(*output, object)
		}
	}
	return false
}

func collectRuntimeModelArray(values []any, output *[]map[string]any) bool {
	for _, nested := range values {
		if collectRuntimeModels(nested, output) {
			return true
		}
	}
	return false
}

func runtimeModelListKey(value string) bool {
	for _, candidate := range []string{"models", "available_models", "model_catalog", "chat_models", "data"} {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func normalizeRuntimeCatalogEntry(object map[string]any) map[string]any {
	id := firstRuntimeString(object, "id", runtimeCatalogModelKey, "slug", "name")
	if id == "" {
		return nil
	}
	displayName := firstRuntimeString(object, "name", "display_name", "label")
	if displayName == "" {
		displayName = id
	}
	maxContext, maxPrompt := runtimeModelLimits(object)
	contextWindow := maxPrompt
	if contextWindow == 0 {
		contextWindow = maxContext
	}
	if contextWindow == 0 {
		contextWindow = 200_000
	}
	entry := map[string]any{
		"id": id, "object": "model", "owned_by": "github-copilot",
		"display_name":                     displayName,
		"description":                      "GitHub Copilot model available for this account: " + displayName + ".",
		"context_window":                   contextWindow,
		"input_cost_per_million_microusd":  uint64(0),
		"output_cost_per_million_microusd": uint64(0),
	}
	addRuntimeModelLimits(entry, maxContext, maxPrompt)
	if capabilities, ok := object[runtimeCatalogCapabilities]; ok {
		entry["capabilities"] = cloneRuntimeValue(capabilities)
	}
	stripRuntimeNulls(entry)
	return entry
}

func runtimeModelLimits(object map[string]any) (uint64, uint64) {
	maxContext := firstRuntimeUint(object, "context_window", "context_window_tokens", "max_context_tokens", "max_input_tokens")
	maxPrompt := firstRuntimeUint(object, runtimeCatalogMaxPromptKey)
	capabilities, ok := object[runtimeCatalogCapabilities].(map[string]any)
	if !ok {
		return maxContext, maxPrompt
	}
	limits, ok := capabilities["limits"].(map[string]any)
	if !ok {
		return maxContext, maxPrompt
	}
	if maxContext == 0 {
		maxContext = firstRuntimeUint(limits, "max_context_window_tokens")
	}
	if maxPrompt == 0 {
		maxPrompt = firstRuntimeUint(limits, runtimeCatalogMaxPromptKey)
	}
	return maxContext, maxPrompt
}

func addRuntimeModelLimits(entry map[string]any, maxContext, maxPrompt uint64) {
	if maxContext > 0 {
		entry["max_context_window"] = maxContext
	}
	if maxPrompt > 0 {
		entry[runtimeCatalogMaxPromptKey] = maxPrompt
	}
}

func firstRuntimeString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func firstRuntimeUint(object map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		switch value := object[key].(type) {
		case float64:
			if value > 1 && value == float64(uint64(value)) {
				return uint64(value)
			}
		case uint64:
			if value > 1 {
				return value
			}
		case int:
			if value > 1 {
				return uint64(value)
			}
		}
	}
	return 0
}

func mergeRuntimeCatalog(static, dynamic []map[string]any) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(static)+len(dynamic))
	seen := make(map[string]bool, len(static)+len(dynamic))
	appendEntry := func(entry map[string]any) error {
		id := runtimeCatalogEntryID(entry)
		if id == "" || seen[strings.ToLower(id)] {
			return nil
		}
		if len(result) >= modelCatalogMaxItems {
			return fmt.Errorf("Copilot model catalog exceeds the hard limit of %d entries", modelCatalogMaxItems)
		}
		seen[strings.ToLower(id)] = true
		result = append(result, cloneRuntimeCatalogEntry(entry))
		return nil
	}
	for _, entry := range static {
		if err := appendEntry(entry); err != nil {
			return nil, err
		}
	}
	for _, entry := range dynamic {
		if err := appendEntry(entry); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func runtimeCatalogEntryID(entry map[string]any) string {
	for _, key := range []string{"id", "slug", runtimeCatalogModelKey} {
		if value, ok := entry[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func cloneRuntimeCatalogEntry(entry map[string]any) map[string]any {
	result := make(map[string]any, len(entry))
	for key, value := range entry {
		result[key] = cloneRuntimeValue(value)
	}
	return result
}

func cloneRuntimeValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		return cloneRuntimeCatalogEntry(current)
	case []any:
		copy := make([]any, len(current))
		for index, item := range current {
			copy[index] = cloneRuntimeValue(item)
		}
		return copy
	case []string:
		return append([]string(nil), current...)
	default:
		return current
	}
}

func stripRuntimeNulls(object map[string]any) {
	for key, value := range object {
		if value == nil {
			delete(object, key)
			continue
		}
		switch nested := value.(type) {
		case map[string]any:
			stripRuntimeNulls(nested)
		case []any:
			for _, item := range nested {
				if mapItem, ok := item.(map[string]any); ok {
					stripRuntimeNulls(mapItem)
				}
			}
		}
	}
}
