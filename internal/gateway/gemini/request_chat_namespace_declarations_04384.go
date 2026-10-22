package gemini

import (
	"strings"
	"unicode"
)

func projectGeminiChatNamespaceDeclarations04384(tool map[string]any) []any {
	namespace, _ := tool["name"].(string)
	if strings.TrimSpace(namespace) == "" {
		return nil
	}
	children, _ := tool["tools"].([]any)
	var result []any
	for _, raw := range children {
		child, _ := raw.(map[string]any)
		name, _ := child["name"].(string)
		if child["type"] != "function" || strings.TrimSpace(name) == "" {
			continue
		}
		function := map[string]any{"name": geminiChatNamespacedTool04384(namespace, name, false)}
		if description, _ := child["description"].(string); strings.TrimSpace(description) != "" {
			function["description"] = description
		}
		if parameters, present := firstPresent(child, "parameters", "parametersJsonSchema", "input_schema", "schema"); present {
			function["parameters"] = parameters
		}
		if strict, ok := child["strict"].(bool); ok {
			function["strict"] = strict
		}
		result = append(result, map[string]any{"type": "function", "function": function})
	}
	return result
}

func validGeminiChatNamespaceFields04384(tool map[string]any) bool {
	if !validGeminiChatFunctionFields04384(tool) {
		return false
	}
	children, ok := tool["tools"].([]any)
	if !ok || len(children) == 0 {
		return false
	}
	for _, raw := range children {
		child, ok := raw.(map[string]any)
		name, _ := child["name"].(string)
		if !ok || child["type"] != "function" || strings.TrimSpace(name) == "" || !validGeminiChatFunctionFields04384(child) {
			return false
		}
	}
	return true
}

func geminiChatNamespacedTool04384(namespace, name string, addMCPPrefix bool) string {
	if addMCPPrefix {
		// Tagged MCP prefixing retains leading namespace whitespace until name validation.
		namespace = strings.TrimRightFunc(namespace, unicode.IsSpace)
	} else {
		namespace = strings.TrimSpace(namespace)
	}
	name = strings.TrimSpace(name)
	separator := "--"
	if (addMCPPrefix || strings.HasPrefix(namespace, "mcp__")) && !strings.HasSuffix(namespace, "_") &&
		!strings.HasPrefix(name, "_") && !strings.Contains(name, "__") {
		separator = "__"
	}
	if addMCPPrefix {
		return "mcp__" + namespace + separator + name
	}
	return namespace + separator + name
}
