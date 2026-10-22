package gemini

import (
	"errors"
	"strings"
)

func geminiChatResponseMetadata04384(body []byte, controls map[string]any) (map[string]any, error) {
	root, err := decodeGeminiChatRequest04384(body)
	if err != nil {
		return nil, err
	}
	metadata, ok := root["metadata"].(map[string]any)
	if _, present := root["metadata"]; present && !ok {
		return nil, errors.New("invalid Gemini request metadata")
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	provider, ok := metadata["gemini"].(map[string]any)
	if _, present := metadata["gemini"]; present && !ok {
		return nil, errors.New("invalid Gemini provider metadata")
	}
	if value, present := root["client_metadata"]; present {
		client, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("invalid Gemini client metadata")
		}
		metadata["client_metadata"] = client
	}
	for _, field := range []string{"prompt_cache_key", "prompt_cache_retention"} {
		if value, present := root[field]; present {
			text, ok := value.(string)
			if !ok {
				return nil, errors.New("invalid Gemini prompt cache metadata")
			}
			if field != "prompt_cache_key" || strings.TrimSpace(text) != "" {
				metadata[field] = text
			}
		}
	}
	geminiChatFormatMetadata04384(root, metadata)
	provider, _ = metadata["gemini"].(map[string]any)
	if choice, present := root["tool_choice"]; present && geminiChatThinking04384(controls) {
		if provider == nil {
			provider = make(map[string]any)
			metadata["gemini"] = provider
		}
		provider["omitted_tool_choice"] = map[string]any{
			"from": choice, "reason": geminiChatThinkingChoiceReason04384,
		}
	}
	return metadata, nil
}
