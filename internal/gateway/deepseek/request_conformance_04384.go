package deepseek

import "strings"

// Tagged local_rewrite_deepseek_send.rs replaces only the emitted body for this policy.
func deepSeekSimpleRequest04384(request map[string]any, contains func(string) bool) bool {
	if id, _ := request["previous_response_id"].(string); strings.TrimSpace(id) != "" && contains(id) {
		return false
	}
	for _, key := range []string{"web_search_options", "safety_identifier"} {
		if _, present := request[key]; present {
			return false
		}
	}
	if value, present := request["response_format"]; present {
		format, ok := value.(map[string]any)
		if !ok {
			return false
		}
		switch format["type"] {
		case "text", "json_object", "json_schema", "json", "structured_output":
		default:
			return false
		}
	}
	if tools, present := request["tools"]; present && !deepSeekSimpleTools04384(tools) {
		return false
	}
	if !deepSeekSimpleChoice04384(request["tool_choice"]) {
		return false
	}
	switch input := request["input"].(type) {
	case string:
		return true
	case []any:
		for _, raw := range input {
			item, ok := raw.(map[string]any)
			if !ok || !deepSeekSimpleItem04384(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func deepSeekSimpleItem04384(item map[string]any) bool {
	if value, present := item["type"]; present {
		switch value {
		case "function_call_output":
			_, output := item["output"]
			return deepSeekSimpleString04384(item, "call_id") && output
		case "mcp_tool_result", "mcp_call_output":
			id := deepSeekSimpleString04384(item, "call_id") || deepSeekSimpleString04384(item, "tool_call_id") || deepSeekSimpleString04384(item, "id")
			return id && deepSeekSimpleCallID04384(item) && deepSeekSimpleResult04384(item)
		case "custom_tool_call_output":
			return deepSeekSimpleString04384(item, "call_id") && deepSeekSimpleResult04384(item)
		case "function_call", "mcp_call":
			return deepSeekSimpleName04384(item, true) && deepSeekSimpleCallID04384(item)
		case "custom_tool_call":
			return deepSeekSimpleName04384(item, false) && deepSeekSimpleCallID04384(item)
		case "local_shell_call":
			action, _ := item["action"].(map[string]any)
			return (deepSeekSimpleString04384(item, "command") || deepSeekSimpleStringArray04384(action["command"])) && deepSeekSimpleCallID04384(item)
		case "message":
		default:
			return false
		}
	}
	if role, present := item["role"]; present {
		switch role {
		case "system", "user", "assistant", "tool":
		default:
			return false
		}
	}
	content, present := item["content"]
	if !present {
		return true
	}
	if _, ok := content.(string); ok {
		return true
	}
	parts, ok := content.([]any)
	if !ok {
		return false
	}
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok || !deepSeekSimpleText04384(part) {
			return false
		}
	}
	return true
}

func deepSeekSimpleText04384(part map[string]any) bool {
	if kind, present := part["type"]; present {
		switch kind {
		case "input_text", "output_text", "text":
		default:
			return false
		}
	}
	for _, key := range []string{"text", "input_text", "output_text"} {
		if _, present := part[key]; present {
			return deepSeekSimpleString04384(part, key)
		}
	}
	return false
}

func deepSeekSimpleTools04384(value any) bool {
	tools, ok := value.([]any)
	if !ok {
		return false
	}
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		name, ok := function["name"].(string)
		if tool["type"] != "function" || !ok || name == "" {
			return false
		}
	}
	return true
}

func deepSeekSimpleChoice04384(value any) bool {
	switch choice := value.(type) {
	case nil:
		return true
	case string:
		return choice == "auto" || choice == "none" || choice == "required"
	case map[string]any:
		name, ok := choice["name"].(string)
		return choice["type"] == "function" && ok && name != ""
	default:
		return false
	}
}

func deepSeekSimpleName04384(item map[string]any, nested bool) bool {
	for _, key := range []string{"name", "tool_name"} {
		if _, present := item[key]; present {
			return deepSeekSimpleString04384(item, key)
		}
	}
	function, _ := item["function"].(map[string]any)
	return nested && deepSeekSimpleString04384(function, "name")
}

func deepSeekSimpleCallID04384(item map[string]any) bool {
	for _, key := range []string{"call_id", "tool_call_id", "id"} {
		if _, present := item[key]; present {
			return deepSeekSimpleString04384(item, key)
		}
	}
	return true
}

func deepSeekSimpleResult04384(item map[string]any) bool {
	for _, key := range []string{"output", "content", "result", "error"} {
		if _, present := item[key]; present {
			return true
		}
	}
	return false
}

func deepSeekSimpleString04384(object map[string]any, key string) bool {
	_, ok := object[key].(string)
	return ok
}

func deepSeekSimpleStringArray04384(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range items {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}
