package deepseek

import (
	"errors"
	"strings"
)

type TranslatedRequest struct {
	Body             []byte
	ResponseMetadata map[string]any
}

func deepSeekResponseMetadata(object map[string]any, thinking bool) (map[string]any, error) {
	metadata, err := requestMetadataObject(object)
	if err != nil {
		return nil, err
	}
	if err := addClientMetadata(metadata, object); err != nil {
		return nil, err
	}
	if err := addCacheMetadata(metadata, object); err != nil {
		return nil, err
	}
	addDegradedResponseFormatMetadata(metadata, object)
	addThinkingToolChoiceMetadata(metadata, object, thinking)
	if len(metadata) == 0 {
		return nil, nil
	}
	return metadata, nil
}

func requestMetadataObject(object map[string]any) (map[string]any, error) {
	value, found := object[deepSeekMetadataKey]
	if !found {
		return make(map[string]any), nil
	}
	metadata, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("DeepSeek request metadata must be an object")
	}
	result := cloneMap(metadata)
	if provider, found := result[deepSeekProviderKey]; found {
		if _, ok := provider.(map[string]any); !ok {
			return nil, errors.New("DeepSeek request metadata.deepseek must be an object")
		}
	}
	return result, nil
}

func addClientMetadata(metadata, object map[string]any) error {
	value, found := object["client_metadata"]
	if !found {
		return nil
	}
	client, ok := value.(map[string]any)
	if !ok {
		return errors.New("DeepSeek client_metadata must be an object")
	}
	metadata["client_metadata"] = cloneMap(client)
	return nil
}

func addCacheMetadata(metadata, object map[string]any) error {
	if value, found := object["prompt_cache_key"]; found {
		text, ok := value.(string)
		if !ok {
			return errors.New("DeepSeek prompt_cache_key must be a string")
		}
		if strings.TrimSpace(text) != "" {
			metadata["prompt_cache_key"] = text
		}
	}
	if value, found := object["prompt_cache_retention"]; found {
		text, ok := value.(string)
		if !ok {
			return errors.New("DeepSeek prompt_cache_retention must be a string")
		}
		metadata["prompt_cache_retention"] = text
	}
	return nil
}

func addDegradedResponseFormatMetadata(metadata, object map[string]any) {
	kind := responseFormatSourceKind(object)
	if kind != "json_schema" && kind != "structured_output" {
		return
	}
	provider := ensureDeepSeekMetadata(metadata)
	provider["degraded_response_format"] = map[string]any{
		"from":   kind,
		"reason": "DeepSeek response_format supports json_object but not native JSON Schema enforcement",
	}
}

func addThinkingToolChoiceMetadata(metadata, object map[string]any, thinking bool) {
	if !thinking {
		return
	}
	choice, found := object["tool_choice"]
	if !found {
		return
	}
	provider := ensureDeepSeekMetadata(metadata)
	provider["omitted_tool_choice"] = map[string]any{
		"from":   choice,
		"reason": "DeepSeek thinking mode currently rejects explicit tool_choice on the OpenAI Chat route, so Godex omits it while preserving translated function tools",
	}
}

func ensureDeepSeekMetadata(metadata map[string]any) map[string]any {
	if existing, ok := metadata[deepSeekProviderKey].(map[string]any); ok {
		return existing
	}
	provider := make(map[string]any)
	metadata[deepSeekProviderKey] = provider
	return provider
}

func responseFormatSourceKind(object map[string]any) string {
	value, found := object["response_format"]
	if !found {
		if text, ok := object["text"].(map[string]any); ok {
			value, found = text["format"]
		}
	}
	if !found {
		return ""
	}
	format, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	kind, _ := format["type"].(string)
	return strings.TrimSpace(kind)
}

func mergeResponseMetadata(response map[string]any, incoming map[string]any) {
	if len(incoming) == 0 {
		return
	}
	existing, ok := response[deepSeekMetadataKey].(map[string]any)
	if !ok {
		existing = make(map[string]any)
		response[deepSeekMetadataKey] = existing
	}
	mergeMetadataObject(existing, incoming)
}

func mergeMetadataObject(target, incoming map[string]any) {
	for key, value := range incoming {
		incomingObject, incomingIsObject := value.(map[string]any)
		targetValue, exists := target[key]
		targetObject, targetIsObject := targetValue.(map[string]any)
		if incomingIsObject && targetIsObject {
			mergeMetadataShallow(targetObject, incomingObject)
			continue
		}
		if incomingIsObject && exists && !targetIsObject {
			continue
		}
		target[key] = cloneMetadataValue(value)
	}
}

func mergeMetadataShallow(target, incoming map[string]any) {
	for key, value := range incoming {
		target[key] = cloneMetadataValue(value)
	}
}

func cloneMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		return append([]any(nil), typed...)
	default:
		return typed
	}
}
