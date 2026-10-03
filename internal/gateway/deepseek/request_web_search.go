package deepseek

import (
	"errors"
	"strings"
)

func deepSeekWebSearchOptions(object map[string]any, mode string) (map[string]any, error) {
	options, err := deepSeekWebSearchOptionsFromRequest(object)
	if err != nil {
		return nil, err
	}
	if options != nil && mode == "off" {
		return nil, errors.New("DeepSeek web search mode is off; remove web_search tools or set deepseek.web_search_mode")
	}
	return options, nil
}

func deepSeekWebSearchOptionsFromRequest(object map[string]any) (map[string]any, error) {
	if value, found := object["web_search_options"]; found {
		options, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("DeepSeek web_search_options must be an object")
		}
		return options, nil
	}
	return deepSeekWebSearchOptionsFromTools(object)
}

func deepSeekWebSearchOptionsFromTools(object map[string]any) (map[string]any, error) {
	tools, _ := object["tools"].([]any)
	var options map[string]any
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok || !deepSeekWebSearchTool(tool) {
			continue
		}
		if err := validateDeepSeekWebSearchToolContextSize(tool); err != nil {
			return nil, err
		}
		if options != nil {
			continue
		}
		options = deepSeekWebSearchOptionsFromTool(tool)
	}
	if options != nil {
		if err := validateDeepSeekWebSearchOptions(options); err != nil {
			return nil, err
		}
	}
	return options, nil
}

func deepSeekWebSearchOptionsFromTool(tool map[string]any) map[string]any {
	options := make(map[string]any)
	size, found := tool["search_context_size"]
	if !found {
		size, found = tool["context_size"]
	}
	if found {
		options["search_context_size"] = size
	}
	for _, key := range []string{"allowed_domains", "blocked_domains", "max_uses"} {
		if value, found := tool[key]; found {
			options[key] = value
		}
	}
	location, found := tool["user_location"]
	if !found {
		location, found = tool["location"]
	}
	if found {
		options["user_location"] = location
	}
	return options
}

func validateDeepSeekWebSearchToolContextSize(tool map[string]any) error {
	value, found := tool["search_context_size"]
	if !found {
		value, found = tool["context_size"]
	}
	if !found {
		return nil
	}
	text, valid := value.(string)
	if !valid || !validDeepSeekSearchContextSize(text) {
		return errors.New("DeepSeek web_search context_size must be low, medium, or high")
	}
	return nil
}

func deepSeekWebSearchTool(tool map[string]any) bool {
	kind, _ := tool["type"].(string)
	return kind == "web_search" || kind == "web_search_preview" || strings.HasPrefix(kind, "web_search_preview_")
}

func validDeepSeekSearchContextSize(value string) bool {
	return value == "low" || value == "medium" || value == "high"
}

func validateDeepSeekWebSearchOptions(options map[string]any) error {
	if value, found := options["search_context_size"]; found {
		text, ok := value.(string)
		if !ok || !validDeepSeekSearchContextSize(text) {
			return errors.New("DeepSeek web_search search_context_size must be low, medium, or high")
		}
	}
	for _, key := range []string{"allowed_domains", "blocked_domains"} {
		if value, found := options[key]; found {
			domains, ok := value.([]any)
			if !ok {
				return errors.New("DeepSeek web_search " + key + " must be an array of strings")
			}
			for _, raw := range domains {
				text, ok := raw.(string)
				if !ok || strings.TrimSpace(text) == "" {
					return errors.New("DeepSeek web_search " + key + " entries must be non-empty strings")
				}
			}
		}
	}
	if value, found := options["max_uses"]; found && !positiveInteger(value) {
		return errors.New("DeepSeek web_search max_uses must be a positive integer")
	}
	if value, found := options["user_location"]; found {
		if _, ok := value.(map[string]any); !ok {
			return errors.New("DeepSeek web_search user_location must be an object")
		}
	}
	return nil
}
