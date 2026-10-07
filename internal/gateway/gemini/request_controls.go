package gemini

import (
	"errors"
	"fmt"
	"strings"
)

func validateInputItems(value any) error {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	for _, item := range items {
		if _, ok := item.(map[string]any); !ok {
			return errors.New("Gemini OpenAI-compatible input items must be objects")
		}
	}
	return nil
}

func geminiReasoningEffort(request map[string]any) (string, error) {
	value, exists := request["reasoning_effort"]
	if reasoning, ok := request["reasoning"].(map[string]any); ok {
		if nested, found := reasoning["effort"]; found {
			value, exists = nested, true
		}
	} else if reasoning, found := request["reasoning"]; found && reasoning != nil {
		return "", errors.New("Gemini OpenAI-compatible reasoning must be an object")
	}
	if !exists {
		return "", nil
	}
	effort, ok := value.(string)
	if !ok {
		return "", errors.New("Gemini OpenAI-compatible reasoning effort must be a string")
	}
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "xhigh", "max", "high":
		return "high", nil
	case "medium", "low", "minimal", "none":
		return strings.ToLower(strings.TrimSpace(effort)), nil
	default:
		return "", errors.New("Gemini OpenAI-compatible reasoning effort is not supported")
	}
}

func reasoningEnabled(effort string) bool { return effort != "" && effort != "none" }

func geminiResponseFormat(request map[string]any) (map[string]any, bool, error) {
	value, exists := request["response_format"]
	if !exists {
		if text, ok := request["text"].(map[string]any); ok {
			value, exists = text["format"]
		}
	}
	if !exists {
		return nil, false, nil
	}
	format, ok := value.(map[string]any)
	if !ok {
		return nil, false, errors.New("Gemini OpenAI-compatible response_format must be an object")
	}
	kind, ok := format["type"].(string)
	if !ok || kind == "" {
		return nil, false, errors.New("Gemini OpenAI-compatible response_format must include a type")
	}
	switch kind {
	case "text":
		return nil, false, nil
	case "json", "json_object":
		return map[string]any{"type": "json_object"}, false, nil
	case "json_schema", "structured_output":
		return map[string]any{"type": "json_object"}, true, nil
	default:
		return nil, false, fmt.Errorf("Gemini response_format type `%s` is not supported", kind)
	}
}

func responseFormatType(request map[string]any) string {
	value, exists := request["response_format"]
	if !exists {
		if text, ok := request["text"].(map[string]any); ok {
			value, exists = text["format"]
		}
	}
	format, ok := value.(map[string]any)
	if !exists || !ok {
		return ""
	}
	kind, ok := format["type"].(string)
	if !ok {
		return ""
	}
	return kind
}

func geminiResponseMetadata(request, gemini map[string]any) (map[string]any, error) {
	metadata, ok := request["metadata"].(map[string]any)
	if value, exists := request["metadata"]; exists {
		metadata, ok = value.(map[string]any)
		if !ok {
			return nil, errors.New("Gemini OpenAI-compatible metadata must be an object")
		}
	}
	result := make(map[string]any, len(metadata)+2)
	for key, value := range metadata {
		result[key] = value
	}
	current, exists := result["gemini"]
	if exists {
		provider, ok := current.(map[string]any)
		if !ok {
			return nil, errors.New("Gemini OpenAI-compatible metadata.gemini must be an object")
		}
		current = provider
	}
	if value, exists := request["client_metadata"]; exists {
		client, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("Gemini OpenAI-compatible client_metadata must be an object")
		}
		result["client_metadata"] = client
	}
	if len(gemini) > 0 || exists {
		provider, _ := current.(map[string]any)
		merged := make(map[string]any, len(provider)+len(gemini))
		for key, value := range provider {
			merged[key] = value
		}
		for key, value := range gemini {
			if _, exists := merged[key]; !exists {
				merged[key] = value
			}
		}
		result["gemini"] = merged
	}
	if value, exists := request["prompt_cache_key"]; exists {
		if key, ok := value.(string); !ok {
			return nil, errors.New("Gemini OpenAI-compatible prompt_cache_key must be a string")
		} else if strings.TrimSpace(key) != "" {
			addGeminiMetadata(result, "prompt_cache_key", key)
		}
	}
	if value, exists := request["prompt_cache_retention"]; exists {
		if retention, ok := value.(string); !ok {
			return nil, errors.New("Gemini OpenAI-compatible prompt_cache_retention must be a string")
		} else {
			addGeminiMetadata(result, "prompt_cache_retention", retention)
		}
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func addGeminiMetadata(metadata map[string]any, key string, value any) {
	provider, _ := metadata["gemini"].(map[string]any)
	if provider == nil {
		provider = make(map[string]any)
		metadata["gemini"] = provider
	}
	if _, exists := provider[key]; !exists {
		provider[key] = value
	}
}
