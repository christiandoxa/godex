package kiro

import "strings"

func kiroChatPromptSection(message map[string]any) string {
	role := runtimeString(message["role"], "message")
	content, _ := runtimeTextContent(message[kiroFieldContent])
	block := strings.TrimSpace(content)
	if calls, ok := message["tool_calls"].([]any); ok {
		for _, raw := range calls {
			call, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			function, _ := call[kiroFieldFunction].(map[string]any)
			name := runtimeString(function["name"], "tool_call")
			arguments := runtimeString(function["arguments"], "{}")
			block = appendKiroPromptLine(block, "Tool call "+name+": "+arguments)
		}
	}
	if function, ok := message[kiroFieldFunctionCall].(map[string]any); ok {
		name := runtimeString(function["name"], kiroFieldFunction)
		arguments := runtimeString(function["arguments"], "{}")
		block = appendKiroPromptLine(block, "Tool call "+name+": "+arguments)
	}
	if strings.TrimSpace(block) == "" {
		return ""
	}
	return runtimeRole(role) + ":\n" + strings.TrimSpace(block)
}

func appendKiroPromptLine(block, line string) string {
	if strings.TrimSpace(line) == "" {
		return block
	}
	if block == "" {
		return line
	}
	return block + "\n" + line
}

func normalizeLegacyKiroFunctionCall(object map[string]any) {
	value, found := object[kiroFieldFunctionCall]
	if !found || value == nil {
		return
	}
	delete(object, kiroFieldFunctionCall)
	if text, ok := value.(string); ok {
		if text == "auto" || text == "none" {
			object["tool_choice"] = text
		}
		return
	}
	function, ok := value.(map[string]any)
	if !ok {
		return
	}
	name := runtimeString(function["name"], "")
	if name == "" {
		return
	}
	object["tool_choice"] = map[string]any{
		"type":            kiroFieldFunction,
		kiroFieldFunction: map[string]any{"name": name},
	}
}
