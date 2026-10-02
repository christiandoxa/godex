package chatcompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

func responseToolItem(raw any, index int, options ResponseOptions) (map[string]any, error) {
	call, ok := raw.(map[string]any)
	if !ok {
		return nil, nil
	}
	function, ok := call["function"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s returned a tool call without a function object", options.AdapterLabel)
	}
	name, _ := function["name"].(string)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%s returned a tool call without a function name", options.AdapterLabel)
	}
	arguments, ok := function[chatArgumentsKey].(string)
	if !ok {
		arguments = "{}"
	}
	if err := validateToolArguments(name, arguments, options.AdapterLabel); err != nil {
		return nil, err
	}
	callID := stringOr(call["id"], options.FallbackCallID(index))
	return typedToolCallItem(call, name, arguments, callID), nil
}

func validateToolArguments(name, arguments, label string) error {
	if strings.TrimSpace(arguments) == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(arguments), &value); err != nil {
		return fmt.Errorf("%s returned malformed JSON arguments for tool call `%s`: %w", label, name, err)
	}
	return nil
}

func typedToolCallItem(call map[string]any, name, arguments, callID string) map[string]any {
	switch name {
	case "tool_search":
		var value any
		_ = json.Unmarshal([]byte(arguments), &value)
		return map[string]any{
			"type": "tool_search_call", chatCallIDKey: callID,
			"execution": "client", chatArgumentsKey: value,
		}
	case "apply_patch":
		return map[string]any{
			"type": "custom_tool_call", chatCallIDKey: callID,
			"name": name, "input": applyPatchInput(arguments),
		}
	}
	arguments = WrapRTKArguments(name, arguments)
	namespace, short := splitToolName(name)
	item := map[string]any{
		"type": "function_call", chatCallIDKey: callID,
		"name": short, chatArgumentsKey: arguments,
	}
	if namespace != "" {
		item["namespace"] = namespace
	}
	if signature := toolThoughtSignature(call); signature != "" {
		item["gemini_thought_signature"] = signature
	}
	return item
}

func applyPatchInput(arguments string) string {
	var value any
	if json.Unmarshal([]byte(arguments), &value) != nil {
		return arguments
	}
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		for _, key := range []string{"input", "patch", "diff", "text", chatContentKey} {
			if text, ok := typed[key].(string); ok {
				return text
			}
		}
	}
	content, err := json.Marshal(value)
	if err != nil {
		return arguments
	}
	return string(content)
}

func splitToolName(name string) (string, string) {
	if namespace, short, ok := strings.Cut(name, "--"); ok && strings.TrimSpace(namespace) != "" && strings.TrimSpace(short) != "" {
		if index := strings.LastIndex(name, "--"); index > 0 && index < len(name)-2 {
			return name[:index], name[index+2:]
		}
	}
	if rest, ok := strings.CutPrefix(name, "mcp__"); ok {
		if index := strings.LastIndex(rest, "__"); index > 0 && index < len(rest)-2 {
			return "mcp__" + rest[:index], rest[index+2:]
		}
	}
	if index := strings.LastIndex(name, "."); index > 0 && index < len(name)-1 {
		return name[:index], name[index+1:]
	}
	return "", name
}

func toolThoughtSignature(call map[string]any) string {
	extra, _ := call["extra_content"].(map[string]any)
	google, _ := extra["google"].(map[string]any)
	signature, _ := google["thought_signature"].(string)
	return strings.TrimSpace(signature)
}
