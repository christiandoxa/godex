package gemini

import "strings"

func projectGeminiChatSearchDeclarations04384(tool map[string]any) []any {
	var result []any
	if name, _ := tool["name"].(string); strings.HasPrefix(name, "mcp__") {
		result = projectGeminiChatMCPDeclarations04384(tool)
	}
	function := map[string]any{"name": "tool_search"}
	if description, _ := tool["description"].(string); strings.TrimSpace(description) != "" {
		function["description"] = description
	}
	if parameters, present := tool["parameters"]; present {
		function["parameters"] = parameters
	}
	return append(result, map[string]any{"type": "function", "function": function})
}
