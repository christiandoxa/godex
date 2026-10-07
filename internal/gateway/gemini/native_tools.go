package gemini

import (
	"encoding/json"
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func geminiNativeToolCallItem(part, call map[string]any, fallbackID string) map[string]any {
	name := nativeString(call["name"])
	if strings.TrimSpace(name) == "" {
		name = "tool_call"
	}
	callID := nativeString(call["id"])
	if strings.TrimSpace(callID) == "" {
		callID = fallbackID
	}
	args := call["args"]
	if args == nil {
		args = map[string]any{}
	}
	if name == "tool_search" {
		return map[string]any{
			"type": "tool_search_call", "call_id": callID,
			"execution": "client", "arguments": args,
		}
	}
	if name == "apply_patch" {
		return map[string]any{
			"type": "custom_tool_call", "call_id": callID,
			"name": name, "input": geminiNativeApplyPatchInput(args),
		}
	}

	encoded, err := json.Marshal(args)
	if err != nil {
		encoded = []byte("{}")
	}
	namespace, short := geminiNativeSplitToolName(name)
	arguments := chatcompat.WrapRTKArguments(short, string(encoded))
	item := map[string]any{
		"type": "function_call", "call_id": callID,
		"name": short, "arguments": arguments,
	}
	if namespace != "" {
		item["namespace"] = namespace
	}
	if signature := geminiNativeThoughtSignature(part, call); signature != "" {
		item["gemini_thought_signature"] = signature
	}
	return item
}

func geminiNativeThoughtSignature(part, call map[string]any) string {
	for _, object := range []map[string]any{part, call} {
		for _, key := range []string{"thoughtSignature", "thought_signature"} {
			if signature := nativeString(object[key]); strings.TrimSpace(signature) != "" {
				return signature
			}
		}
	}
	return ""
}

func geminiNativeSplitToolName(name string) (string, string) {
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
	if index := strings.LastIndex(name, "."); index > 0 && index+1 < len(name) {
		return name[:index], name[index+1:]
	}
	return "", name
}

func geminiNativeApplyPatchInput(value any) string {
	if input, ok := geminiNativeExplicitPatchInput(value); ok {
		return geminiNativeNormalizePatch(input)
	}
	if input, ok := geminiNativeEditPatch(value); ok {
		return input
	}
	if object, ok := value.(map[string]any); ok {
		if content, ok := object["content"].(string); ok {
			return geminiNativeNormalizePatch(content)
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return geminiNativeNormalizePatch(string(encoded))
}

func geminiNativeExplicitPatchInput(value any) (string, bool) {
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

func geminiNativeEditPatch(value any) (string, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	path := geminiNativeFirstString(object, "file_path", "path", "filename")
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	old, oldOK := geminiNativeFirstStringFound(object, "old_string", "old", "search", "find")
	next, nextOK := geminiNativeFirstStringFound(object, "new_string", "new", "replace", "replacement", "content")
	if !nextOK {
		return "", false
	}
	path = strings.TrimSpace(path)
	if !oldOK || old == "" {
		lines := []string{"*** Begin Patch", "*** Add File: " + path}
		if next == "" {
			lines = append(lines, "+")
		} else {
			for _, line := range strings.Split(geminiNativeNewlines(next), "\n") {
				lines = append(lines, "+"+line)
			}
		}
		lines = append(lines, "*** End Patch")
		return strings.Join(lines, "\n"), true
	}
	lines := []string{"*** Begin Patch", "*** Update File: " + path, "@@"}
	for _, line := range strings.Split(geminiNativeNewlines(old), "\n") {
		lines = append(lines, "-"+line)
	}
	for _, line := range strings.Split(geminiNativeNewlines(next), "\n") {
		lines = append(lines, "+"+line)
	}
	lines = append(lines, "*** End Patch")
	return strings.Join(lines, "\n"), true
}

func geminiNativeNormalizePatch(input string) string {
	input = geminiNativeNewlines(input)
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

func geminiNativeNewlines(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
}

func geminiNativeFirstString(object map[string]any, keys ...string) string {
	value, _ := geminiNativeFirstStringFound(object, keys...)
	return value
}

func geminiNativeFirstStringFound(object map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := object[key].(string); ok {
			return value, true
		}
	}
	return "", false
}

func geminiMaxTokensIncompleteDetails() map[string]any {
	return map[string]any{
		"reason":  "max_output_tokens",
		"message": "Gemini stopped because it reached the maximum output token limit.",
	}
}
