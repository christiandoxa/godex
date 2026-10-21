package deepseek

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func deepSeekStreamAddedToolItem(callID, name string) map[string]any {
	switch name {
	case "tool_search":
		return nil
	case "apply_patch":
		return map[string]any{
			"type": "custom_tool_call", "call_id": callID,
			"name": name, "input": "",
		}
	}
	namespace, shortName := deepSeekStreamSplitToolName(name)
	item := map[string]any{
		"type": "function_call", "call_id": callID,
		// Prodex 0.437.13 introduces arguments only in the subsequent
		// function_call_arguments.delta and completed tool item.
		"name": shortName,
	}
	if namespace != "" {
		item["namespace"] = namespace
	}
	return item
}

func deepSeekStreamCompletedToolItem(call *deepSeekChatStreamToolCall) map[string]any {
	if call == nil {
		return nil
	}
	arguments := call.arguments.String()
	switch call.name {
	case "tool_search":
		var value any
		if json.Unmarshal([]byte(arguments), &value) != nil {
			value = map[string]any{}
		}
		return map[string]any{
			"type": "tool_search_call", "call_id": call.id,
			"execution": "client", "arguments": value,
		}
	case "apply_patch":
		return map[string]any{
			"type": "custom_tool_call", "call_id": call.id,
			"name": call.name, "input": deepSeekStreamApplyPatchInput(arguments),
		}
	}
	arguments = chatcompat.WrapRTKArguments(call.name, arguments)
	call.finalArguments = arguments
	namespace, shortName := deepSeekStreamSplitToolName(call.name)
	item := map[string]any{
		"type": "function_call", "call_id": call.id,
		"name": shortName, "arguments": arguments,
	}
	if namespace != "" {
		item["namespace"] = namespace
	}
	if call.thoughtSignature != "" {
		item["gemini_thought_signature"] = call.thoughtSignature
	}
	return item
}

func deepSeekValidateStreamToolCall(index uint64, call *deepSeekChatStreamToolCall) error {
	if call == nil || strings.TrimSpace(call.name) == "" {
		return fmt.Errorf("DeepSeek streamed a tool call without a function name at index %d", index)
	}
	arguments := call.arguments.String()
	if strings.TrimSpace(arguments) == "" {
		return nil
	}
	// The tagged Prodex Mojo/serde_json boundary deliberately exposes one
	// stable provider error, not Go-specific decoder/Unicode diagnostics.
	invalid := func() error {
		return fmt.Errorf("DeepSeek streamed malformed JSON arguments for tool call `%s` at index %d: invalid JSON", call.name, index)
	}
	if jsonutil.CheckSerdeBoundary(arguments) != nil {
		return invalid()
	}
	var value any
	if json.Unmarshal([]byte(arguments), &value) != nil {
		return invalid()
	}
	return nil
}

func deepSeekStreamApplyPatchInput(arguments string) string {
	var value any
	if json.Unmarshal([]byte(arguments), &value) != nil {
		return arguments
	}
	if input, ok := deepSeekExplicitApplyPatchInput(value); ok {
		return deepSeekNormalizeApplyPatchInput(input)
	}
	if input, ok := deepSeekEditArgsToApplyPatch(value); ok {
		return input
	}
	if object, ok := value.(map[string]any); ok {
		if content, ok := object["content"].(string); ok {
			return deepSeekNormalizeApplyPatchInput(content)
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return arguments
	}
	return deepSeekNormalizeApplyPatchInput(string(encoded))
}

func deepSeekExplicitApplyPatchInput(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case map[string]any:
		for _, key := range []string{"input", "patch", "diff", "text"} {
			if text, ok := typed[key].(string); ok {
				return text, true
			}
		}
	}
	return "", false
}

func deepSeekNormalizeApplyPatchInput(input string) string {
	input = strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(input, "\n")
	start, end := -1, -1
	for index, line := range lines {
		if start < 0 && strings.HasPrefix(strings.TrimLeft(line, " \t"), "*** Begin Patch") {
			start = index
		}
		if start >= 0 && strings.HasPrefix(strings.TrimLeft(line, " \t"), "*** End Patch") {
			end = index
			break
		}
	}
	if start < 0 || end < start {
		return input
	}
	selected := append([]string(nil), lines[start:end+1]...)
	inAddFile := false
	for index, line := range selected {
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			inAddFile = true
		case strings.HasPrefix(line, "*** "):
			inAddFile = false
		case inAddFile && !strings.HasPrefix(line, "+"):
			selected[index] = "+" + line
		}
	}
	return strings.Join(selected, "\n")
}

func deepSeekEditArgsToApplyPatch(value any) (string, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	path := firstDeepSeekString(object, "file_path", "path", "filename")
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	old, oldOK := firstDeepSeekStringFound(object, "old_string", "old", "search", "find")
	newValue, newOK := firstDeepSeekStringFound(object, "new_string", "new", "replace", "replacement", "content")
	if !newOK {
		return "", false
	}
	path = strings.TrimSpace(path)
	if !oldOK || old == "" {
		lines := []string{"*** Begin Patch", "*** Add File: " + path}
		if newValue == "" {
			lines = append(lines, "+")
		} else {
			for _, line := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(newValue, "\r\n", "\n"), "\r", "\n"), "\n") {
				lines = append(lines, "+"+line)
			}
		}
		lines = append(lines, "*** End Patch")
		return strings.Join(lines, "\n"), true
	}
	lines := []string{"*** Begin Patch", "*** Update File: " + path, "@@"}
	for _, line := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(old, "\r\n", "\n"), "\r", "\n"), "\n") {
		lines = append(lines, "-"+line)
	}
	for _, line := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(newValue, "\r\n", "\n"), "\r", "\n"), "\n") {
		lines = append(lines, "+"+line)
	}
	lines = append(lines, "*** End Patch")
	return strings.Join(lines, "\n"), true
}

func firstDeepSeekString(object map[string]any, keys ...string) string {
	value, _ := firstDeepSeekStringFound(object, keys...)
	return value
}

func firstDeepSeekStringFound(object map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := object[key].(string); ok {
			return value, true
		}
	}
	return "", false
}

func (state *deepSeekChatStreamState) observeToolCallDelta(call map[string]any) ([][]byte, bool) {
	index := integerValue(call["index"])
	current := state.toolCalls[index]
	function, hasFunction := call["function"].(map[string]any)
	if current == nil && !hasFunction {
		failed, _, _ := state.failed("invalid_tool_call_arguments", "DeepSeek streamed a tool call without a function object")
		return [][]byte{failed}, true
	}
	if current == nil {
		current = &deepSeekChatStreamToolCall{id: deepSeekCallFallbackID()}
		state.toolCalls[index] = current
	}
	if id, _ := call["id"].(string); strings.TrimSpace(id) != "" {
		current.id = strings.TrimSpace(id)
	}
	if hasFunction {
		if name, _ := function["name"].(string); strings.TrimSpace(name) != "" {
			current.name = strings.TrimSpace(name)
		}
	}
	if signature := deepSeekStreamThoughtSignature(call); signature != "" {
		current.thoughtSignature = signature
	}

	name := current.name
	if name == "" {
		name = "tool_call"
	}
	var events [][]byte
	if !current.added {
		current.added = true
		if item := deepSeekStreamAddedToolItem(current.id, name); item != nil {
			events = append(events, anthropicStreamEvent("response.output_item.added", map[string]any{
				"type": "response.output_item.added", "sequence_number": state.nextSequenceNumber(),
				"output_index": index, "item": item,
			}))
		}
	}
	if hasFunction {
		if arguments, ok := function["arguments"].(string); ok && arguments != "" {
			current.arguments.WriteString(arguments)
			if name != "tool_search" && name != "apply_patch" {
				events = append(events, anthropicStreamEvent("response.function_call_arguments.delta", map[string]any{
					"type": "response.function_call_arguments.delta", "sequence_number": state.nextSequenceNumber(),
					"output_index": index, "call_id": current.id, "delta": arguments,
				}))
			}
		}
	}
	return events, false
}

func deepSeekStreamThoughtSignature(call map[string]any) string {
	extra, _ := call["extra_content"].(map[string]any)
	google, _ := extra["google"].(map[string]any)
	signature, _ := google["thought_signature"].(string)
	return strings.TrimSpace(signature)
}
