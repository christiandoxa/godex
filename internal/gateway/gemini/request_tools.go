package gemini

import (
	"errors"
	"fmt"
	"strings"
)

func geminiTools(value any) ([]any, map[string]bool, error) {
	if value == nil {
		return nil, nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, nil, errors.New("invalid_tool_declaration: Gemini request field `tools` must be an array")
	}
	tools := make([]any, 0, len(items))
	names := make(map[string]bool, len(items))
	for index, raw := range items {
		if err := validateGeminiToolDeclaration(raw, index); err != nil {
			return nil, nil, err
		}
		if err := appendGeminiTool(&tools, names, raw, ""); err != nil {
			return nil, nil, err
		}
		if len(tools) > 128 {
			return nil, nil, errors.New("Gemini OpenAI-compatible supports at most 128 function tools")
		}
	}
	return tools, names, nil
}

func validateGeminiToolDeclaration(raw any, index int) error {
	item, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid_tool_declaration: Gemini request field `tools[%d]` must be an object", index)
	}
	kind, _ := item["type"].(string)
	_, hasFunction := item["function"]
	if kind != "function" && !hasFunction {
		return nil
	}

	function := item
	field := fmt.Sprintf("tools[%d]", index)
	if hasFunction {
		nested, ok := item["function"].(map[string]any)
		if !ok {
			return fmt.Errorf("invalid_tool_declaration: Gemini request field `tools[%d].function` must be an object", index)
		}
		function = nested
		field += ".function"
	}
	name, ok := function["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return fmt.Errorf("invalid_tool_declaration: Gemini request field `%s.name` must be a non-empty string", field)
	}
	parameters, exists := function["parameters"]
	if !exists {
		return fmt.Errorf("invalid_tool_declaration: Gemini request field `%s.parameters` is required", field)
	}
	if _, ok := parameters.(map[string]any); !ok {
		return fmt.Errorf("invalid_tool_declaration: Gemini request field `%s.parameters` must be an object", field)
	}
	if description, exists := function["description"]; exists && description != nil {
		if _, ok := description.(string); !ok {
			return fmt.Errorf("invalid_tool_declaration: Gemini request field `%s.description` must be a string", field)
		}
	}
	return nil
}

func appendGeminiTool(tools *[]any, names map[string]bool, raw any, namespace string) error {
	item, ok := raw.(map[string]any)
	if !ok {
		return errors.New("Gemini OpenAI-compatible tools must contain objects")
	}
	kind, _ := item["type"].(string)
	if kind == "web_search" || kind == "web_search_preview" || strings.HasPrefix(kind, "web_search_preview_") {
		return nil
	}
	if kind == "namespace" {
		name, _ := item["name"].(string)
		children, ok := item["tools"].([]any)
		if name == "" || !ok {
			return errors.New("Gemini OpenAI-compatible namespace tools require a name and tools array")
		}
		if namespace != "" {
			name = toolNameSegment(namespace) + "__" + toolNameSegment(name)
		}
		for _, child := range children {
			if err := appendGeminiTool(tools, names, child, name); err != nil {
				return err
			}
		}
		return nil
	}
	if kind == "mcp_toolset" || (kind == "mcp" && (item["allowed_tools"] != nil || item["configs"] != nil)) {
		return appendMCPToolset(tools, names, item)
	}
	function, _ := item["function"].(map[string]any)
	name, _ := item["name"].(string)
	if name == "" {
		name, _ = function["name"].(string)
	}
	if name == "" && kind == "tool_search" {
		name = "tool_search"
	}
	if namespace != "" {
		name = namespace + "__" + name
	} else if ns, _ := item["namespace"].(string); ns != "" {
		name = toolNameSegment(ns) + "__" + toolNameSegment(name)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("Gemini OpenAI-compatible %s tools require a name", toolKind(kind))
	}
	if len(name) > 128 {
		return errors.New("Gemini OpenAI-compatible function tool names must be at most 128 bytes")
	}
	if names[name] {
		return fmt.Errorf("Gemini OpenAI-compatible function tool name %q is duplicated", name)
	}
	names[name] = true
	var schema any = map[string]any{"type": "object"}
	if kind == "custom" {
		schema = map[string]any{
			"type":       "object",
			"properties": map[string]any{"input": map[string]any{"type": "string", "description": "Exact raw input for this custom/freeform tool."}},
			"required":   []any{"input"}, "additionalProperties": false,
		}
	} else if kind == "tool_search" {
		schema = map[string]any{
			"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}},
			"required": []any{"query"},
		}
	} else if params, exists := firstPresent(item, "parameters", "parametersJsonSchema", "input_schema", "schema"); exists {
		schema = params
	} else if params, exists := firstPresent(function, "parameters", "parametersJsonSchema", "input_schema", "schema"); exists {
		schema = params
	}
	converted := map[string]any{"name": name, "parameters": schema}
	for _, key := range []string{"description", "strict"} {
		if value, exists := item[key]; exists {
			if key == "strict" {
				if _, ok := value.(bool); !ok {
					continue
				}
			}
			converted[key] = value
		} else if value, exists := function[key]; exists {
			if key == "strict" {
				if _, ok := value.(bool); !ok {
					continue
				}
			}
			converted[key] = value
		}
	}
	if kind == "custom" {
		if _, exists := converted["description"]; !exists {
			converted["description"] = "Freeform custom tool input. Call the tool with exact raw input in the `input` string field."
		}
	} else if kind != "function" && !strings.HasPrefix(kind, "mcp") && kind != "tool_search" {
		return fmt.Errorf("Gemini OpenAI-compatible tool type %q is not supported", kind)
	}
	*tools = append(*tools, map[string]any{"type": "function", "function": converted})
	return nil
}

func firstPresent(object map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, exists := object[key]; exists {
			return value, true
		}
	}
	return nil, false
}

func geminiWebSearchOptions(value any) map[string]any {
	items, _ := value.([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		kind, _ := item["type"].(string)
		if kind != "web_search" && kind != "web_search_preview" && !strings.HasPrefix(kind, "web_search_preview_") {
			continue
		}
		options := make(map[string]any)
		for _, key := range []string{"search_context_size", "allowed_domains", "blocked_domains", "max_uses", "user_location"} {
			field, exists := item[key]
			if !exists && key == "search_context_size" {
				field, exists = item["context_size"]
			}
			if !exists && key == "user_location" {
				field, exists = item["location"]
			}
			if key == "search_context_size" {
				size, _ := field.(string)
				if size != "low" && size != "medium" && size != "high" {
					continue
				}
			}
			if exists {
				options[key] = field
			}
		}
		if len(options) > 0 {
			return options
		}
	}
	return nil
}

func toolKind(kind string) string {
	if kind == "" {
		return "function"
	}
	return kind
}

func geminiToolChoice(value any, names map[string]bool) (any, error) {
	if value == nil {
		return nil, nil
	}
	if choice, ok := value.(string); ok {
		if choice == "auto" || choice == "none" || choice == "required" {
			return choice, nil
		}
		return nil, fmt.Errorf("Gemini OpenAI-compatible tool_choice %q is not supported", choice)
	}
	choice, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("Gemini OpenAI-compatible tool_choice must be a string or object")
	}
	kind, _ := choice["type"].(string)
	if kind != "function" && !strings.HasPrefix(kind, "mcp") {
		return nil, errors.New("Gemini OpenAI-compatible named tool_choice requires a function name")
	}
	name := geminiToolChoiceName(choice, kind)
	if name == "" {
		return nil, errors.New("Gemini OpenAI-compatible named tool_choice requires a function name")
	}
	if !names[name] {
		return nil, fmt.Errorf("Gemini OpenAI-compatible tool_choice %q does not match a function tool", name)
	}
	return map[string]any{"type": "function", "function": map[string]any{"name": name}}, nil
}
