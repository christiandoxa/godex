package deepseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

const (
	deepSeekResponseNameMaxBytes      = 4 << 20
	deepSeekResponseArgumentsMaxBytes = 16 << 20
)

func deepSeekChatResponse(body []byte, now time.Time) ([]byte, error) {
	translated, err := chatcompat.ChatResponseWithOptions(body, now, deepSeekResponseOptions())
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(translated, &result); err != nil {
		return nil, errors.New("failed to parse translated DeepSeek Responses JSON")
	}
	var source map[string]any
	if err := json.Unmarshal(body, &source); err != nil {
		return nil, errors.New("failed to parse DeepSeek response JSON")
	}

	output := responseMessages(result["output"])
	if message := deepSeekResponseMessage(source); message != nil {
		if calls, ok := message["tool_calls"].([]any); ok {
			for _, call := range calls {
				item, toolErr := deepSeekResponseToolItem(call)
				if toolErr != nil {
					result["output"] = output
					result["status"] = "failed"
					result["error"] = map[string]any{"code": "invalid_tool_call_arguments", "message": toolErr.Error()}
					return json.Marshal(result)
				}
				output = append(output, item)
			}
		}
	}
	result["output"] = output
	delete(result, "status")
	delete(result, "error")
	return json.Marshal(result)
}

func responseMessages(value any) []any {
	items, _ := value.([]any)
	messages := make([]any, 0, len(items))
	for _, item := range items {
		object, _ := item.(map[string]any)
		if object["type"] == "message" {
			messages = append(messages, item)
		}
	}
	return messages
}

func deepSeekResponseMessage(root map[string]any) map[string]any {
	choices, _ := root["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	return message
}

func deepSeekResponseToolItem(raw any) (map[string]any, error) {
	call, name, arguments, err := deepSeekResponseToolCall(raw)
	if err != nil {
		return nil, err
	}
	if item, handled, err := deepSeekResponseSpecialTool(call, name, arguments); handled || err != nil {
		return item, err
	}
	if name == "shell" || name == "exec" || name == "functions.exec_command" {
		arguments = chatcompat.WrapRTKArguments(name, arguments)
	}
	namespace, shortName := deepSeekSplitToolName(name)
	item := map[string]any{
		"type": "function_call", "call_id": deepSeekResponseCallID(call),
		"name": shortName, "arguments": arguments,
	}
	if namespace != "" {
		item["namespace"] = namespace
	}
	if signature := deepSeekResponseThoughtSignature(call); signature != "" {
		item["gemini_thought_signature"] = signature
	}
	return item, nil
}

func deepSeekResponseToolCall(raw any) (map[string]any, string, string, error) {
	call, ok := raw.(map[string]any)
	if !ok {
		return nil, "", "", errors.New("DeepSeek returned a tool call without a function object")
	}
	function, ok := call["function"].(map[string]any)
	if !ok {
		return nil, "", "", errors.New("DeepSeek returned a tool call without a function object")
	}
	name, ok := function["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return nil, "", "", errors.New("DeepSeek returned a tool call without a function name")
	}
	if len(name) > deepSeekResponseNameMaxBytes {
		return nil, "", "", errors.New("DeepSeek returned a tool call name exceeding the supported size")
	}
	arguments, ok := function["arguments"].(string)
	if !ok {
		arguments = "{}"
	}
	if len(arguments) > deepSeekResponseArgumentsMaxBytes {
		return nil, "", "", errors.New("DeepSeek returned JSON arguments exceeding the supported size")
	}
	return call, name, arguments, nil
}

func deepSeekResponseSpecialTool(call map[string]any, name, arguments string) (map[string]any, bool, error) {
	if strings.TrimSpace(arguments) != "" || name == "tool_search" {
		var decoded any
		if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
			return nil, true, fmt.Errorf("DeepSeek returned malformed JSON arguments for tool call `%s`: %w", name, err)
		}
		if name == "tool_search" {
			return map[string]any{
				"type": "tool_search_call", "call_id": deepSeekResponseCallID(call),
				"execution": "client", "arguments": decoded,
			}, true, nil
		}
	}
	if name == "apply_patch" {
		return map[string]any{
			"type": "custom_tool_call", "call_id": deepSeekResponseCallID(call),
			"name": name, "input": arguments,
		}, true, nil
	}
	return nil, false, nil
}

func deepSeekResponseCallID(call map[string]any) string {
	if id, ok := call["id"].(string); ok {
		return id
	}
	return deepSeekCallFallbackID()
}

func deepSeekSplitToolName(name string) (string, string) {
	name = strings.TrimSpace(name)
	for _, separator := range []string{"__", ".", "/"} {
		index := strings.Index(name, separator)
		if index < 0 {
			continue
		}
		namespace := strings.TrimSpace(name[:index])
		shortName := strings.TrimSpace(name[index+len(separator):])
		if namespace != "" && shortName != "" {
			return namespace, shortName
		}
	}
	return "", name
}

func deepSeekStreamSplitToolName(name string) (string, string) {
	name = strings.TrimSpace(name)
	if index := strings.LastIndex(name, "--"); index > 0 && index+2 < len(name) {
		return name[:index], name[index+2:]
	}
	if strings.HasPrefix(name, "mcp__") {
		rest := strings.TrimPrefix(name, "mcp__")
		if index := strings.LastIndex(rest, "__"); index > 0 && index+2 < len(rest) {
			return "mcp__" + rest[:index], rest[index+2:]
		}
	}
	return "", name
}

func deepSeekResponseThoughtSignature(call map[string]any) string {
	extra, _ := call["extra_content"].(map[string]any)
	google, _ := extra["google"].(map[string]any)
	signature, _ := google["thought_signature"].(string)
	if strings.TrimSpace(signature) == "" {
		return ""
	}
	return signature
}
