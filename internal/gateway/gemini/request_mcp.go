package gemini

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

func appendMCPToolset(tools *[]any, names map[string]bool, item map[string]any) error {
	server := firstString(item, "mcp_server_name", "server_label", "server_name", "name")
	server = toolNameSegment(server)
	if server == "" {
		return errors.New("Gemini OpenAI-compatible MCP toolset requires a server name")
	}
	for _, toolName := range mcpToolNames(item) {
		name := toolNameSegment(toolName)
		if strings.HasPrefix(toolName, "mcp__") {
			name = toolName
		} else {
			name = "mcp__" + server + "__" + name
		}
		if name == "" {
			continue
		}
		description := firstString(item, "description")
		if description == "" {
			description = "MCP tool " + toolName + " from " + server + "."
		}
		if err := appendFunctionTool(tools, names, name, description, map[string]any{"type": "object", "additionalProperties": true}); err != nil {
			return err
		}
	}
	return nil
}

func mcpToolNames(item map[string]any) []string {
	var names []string
	if allowed, ok := item["allowed_tools"].([]any); ok {
		for _, raw := range allowed {
			if name, ok := raw.(string); ok && strings.TrimSpace(name) != "" {
				names = append(names, strings.TrimSpace(name))
			}
		}
	}
	defaultConfig, _ := item["default_config"].(map[string]any)
	defaultEnabled := defaultConfig["enabled"] != false
	if configs, ok := item["configs"].(map[string]any); ok {
		for name, raw := range configs {
			config, _ := raw.(map[string]any)
			enabled, configured := config["enabled"].(bool)
			if enabled || (!configured && defaultEnabled) {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return compactStrings(names)
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func toolNameSegment(value string) string {
	var segment strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			segment.WriteRune(char)
		}
	}
	return segment.String()
}

func appendFunctionTool(tools *[]any, names map[string]bool, name, description string, schema any) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("Gemini OpenAI-compatible function tool requires a name")
	}
	if len(name) > 128 {
		return errors.New("Gemini OpenAI-compatible function tool names must be at most 128 bytes")
	}
	if names[name] {
		return fmt.Errorf("Gemini OpenAI-compatible function tool name %q is duplicated", name)
	}
	names[name] = true
	function := map[string]any{"name": name, "parameters": schema}
	if strings.TrimSpace(description) != "" {
		function["description"] = description
	}
	*tools = append(*tools, map[string]any{"type": "function", "function": function})
	return nil
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func geminiToolChoiceName(choice map[string]any, kind string) string {
	function, _ := choice["function"].(map[string]any)
	name := firstString(choice, "name")
	if name == "" {
		name = firstString(function, "name")
	}
	namespace := firstString(choice, "namespace", "server_label", "mcp_server_name", "server_name")
	if namespace == "" {
		namespace = firstString(function, "namespace")
	}
	if namespace != "" && !strings.HasPrefix(name, "mcp__") {
		if strings.HasPrefix(namespace, "mcp__") {
			name = namespace + "__" + name
		} else if strings.HasPrefix(kind, "mcp") {
			name = "mcp__" + namespace + "__" + name
		} else {
			name = namespace + "__" + name
		}
	}
	return name
}
