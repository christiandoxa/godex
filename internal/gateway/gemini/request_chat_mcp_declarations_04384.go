package gemini

import "strings"

func projectGeminiChatMCPAlias04384(tool map[string]any) []any {
	name, _ := tool["name"].(string)
	if !strings.HasPrefix(name, "mcp__") {
		return nil
	}
	return projectGeminiChatMCPDeclarations04384(tool)
}

func projectGeminiChatMCPDeclarations04384(tool map[string]any) []any {
	kind, _ := tool["type"].(string)
	var result []any
	name, _ := tool["name"].(string)
	if parameters, present := firstPresent(tool, "parameters", "parametersJsonSchema", "input_schema", "schema"); present && strings.TrimSpace(name) != "" {
		function := map[string]any{"name": name, "parameters": parameters}
		if description, _ := tool["description"].(string); strings.TrimSpace(description) != "" {
			function["description"] = description
		}
		if strict, ok := tool["strict"].(bool); ok {
			function["strict"] = strict
		}
		result = append(result, map[string]any{"type": "function", "function": function})
	}
	if kind != "mcp" && kind != "mcp_toolset" {
		return result
	}
	server, _ := geminiNativeFirstStringFound(tool, "mcp_server_name", "server_label", "server_name", "name")
	segment := geminiChatMCPNameSegment04384(server)
	if segment == "" {
		return result
	}
	for _, raw := range mcpToolNames(tool) {
		name := geminiChatMCPNameSegment04384(raw)
		if name == "" {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(raw), "mcp__") {
			name = "mcp__" + segment + "__" + name
		}
		description, _ := tool["description"].(string)
		if strings.TrimSpace(description) == "" {
			description = "MCP tool " + raw + " from " + server + "."
		}
		result = append(result, map[string]any{"type": "function", "function": map[string]any{
			"name": name, "description": description,
			"parameters": map[string]any{"type": "object", "additionalProperties": true},
		}})
	}
	return result
}

func geminiChatMCPNameSegment04384(value string) string {
	alphanumeric := func(char rune) bool {
		return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
	}
	value = strings.TrimFunc(value, func(char rune) bool { return !alphanumeric(char) })
	return strings.Map(func(char rune) rune {
		if alphanumeric(char) || char == '_' {
			return char
		}
		return '_'
	}, value)
}
