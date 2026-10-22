package gemini

import (
	"strings"
)

func normalizeGeminiChatChoice04384(root map[string]any) bool {
	value, present := root["tool_choice"]
	if !present {
		return true
	}
	if choice, ok := value.(string); ok {
		return choice == "auto" || choice == "none" || choice == "required"
	}
	choice, ok := value.(map[string]any)
	if !ok {
		return false
	}
	name, valid := geminiChatChoiceName04384(choice)
	if !valid {
		return false
	}
	tools, _ := root["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if tool["type"] != "function" {
			continue
		}
		function, ok := tool["function"].(map[string]any)
		if !ok {
			function = tool
		}
		if function["name"] == name {
			root["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": name}}
			return true
		}
	}
	return false
}

func geminiChatChoiceName04384(choice map[string]any) (string, bool) {
	kind, _ := choice["type"].(string)
	if kind != "function" && !strings.HasPrefix(kind, "mcp") {
		return "", false
	}
	function, _ := choice["function"].(map[string]any)
	name, found := choice["name"].(string)
	if !found {
		name, _ = function["name"].(string)
	}
	if strings.TrimSpace(name) == "" {
		return "", false
	}
	namespace, found := geminiNativeFirstStringFound(choice, "namespace", "server_label", "mcp_server_name", "server_name")
	if !found {
		namespace, _ = function["namespace"].(string)
	}
	if strings.TrimSpace(namespace) != "" {
		name = geminiChatNamespacedTool04384(namespace, name, strings.HasPrefix(kind, "mcp") && !strings.HasPrefix(namespace, "mcp__"))
	}
	return name, validGeminiChatFunctionName04384(name)
}
