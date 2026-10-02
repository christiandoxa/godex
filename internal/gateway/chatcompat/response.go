package chatcompat

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func ChatResponse(body []byte, now time.Time) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, errors.New("failed to parse chat completions response JSON")
	}
	result := map[string]any{
		"id":         stringOr(root["id"], "resp_prodex"),
		"object":     "response",
		"created_at": uintValueOr(root["created"], uint64(now.Unix())),
		"model":      stringOr(root["model"], "unknown"),
		"output":     []any{},
	}
	output := make([]any, 0)
	if choice := firstChoice(root); choice != nil {
		if message, ok := choice["message"].(map[string]any); ok {
			output = append(output, responseTextItems(message)...)
			output = append(output, responseToolItems(message)...)
		}
	}
	result["output"] = output
	if usage := chatUsage(root["usage"]); usage != nil {
		result["usage"] = usage
	}
	return json.Marshal(result)
}

func firstChoice(root map[string]any) map[string]any {
	choices, ok := root["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	return choice
}

func responseTextItems(message map[string]any) []any {
	texts := chatContentTexts(message["content"])
	if len(texts) == 0 {
		return nil
	}
	parts := make([]any, 0, len(texts))
	for _, text := range texts {
		parts = append(parts, map[string]any{"type": "output_text", "text": text})
	}
	return []any{map[string]any{"type": "message", "role": "assistant", "content": parts}}
}

func chatContentTexts(value any) []string {
	switch content := value.(type) {
	case string:
		if content != "" {
			return []string{content}
		}
	case []any:
		return chatContentPartTexts(content)
	}
	return nil
}

func chatContentPartTexts(parts []any) []string {
	texts := make([]string, 0, len(parts))
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if text := chatContentPartText(part); text != "" {
			texts = append(texts, text)
		}
	}
	return texts
}

func chatContentPartText(part map[string]any) string {
	if text, ok := part["text"].(string); ok && text != "" {
		return text
	}
	text, _ := part["content"].(string)
	return text
}

func responseToolItems(message map[string]any) []any {
	calls, ok := message["tool_calls"].([]any)
	if !ok {
		return nil
	}
	result := make([]any, 0, len(calls))
	for _, raw := range calls {
		call, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		function, ok := call["function"].(map[string]any)
		if !ok {
			continue
		}
		name, _ := function["name"].(string)
		arguments, _ := function["arguments"].(string)
		arguments = WrapRTKArguments(name, arguments)
		namespace, short := splitToolName(name)
		item := map[string]any{
			"type": "function_call", "call_id": stringOr(call["id"], ""),
			"name": short, "arguments": arguments,
		}
		if namespace != "" {
			item["namespace"] = namespace
		}
		result = append(result, item)
	}
	return result
}

func splitToolName(name string) (string, string) {
	index := strings.LastIndex(name, ".")
	if index <= 0 || index == len(name)-1 {
		return "", name
	}
	return name[:index], name[index+1:]
}

func chatUsage(value any) map[string]any {
	usage, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	input := uintValueOr(usage["prompt_tokens"], 0)
	output := uintValueOr(usage["completion_tokens"], 0)
	return map[string]any{"input_tokens": input, "output_tokens": output, "total_tokens": input + output}
}

func stringOr(value any, fallback string) string {
	text, ok := value.(string)
	if !ok || text == "" {
		return fallback
	}
	return text
}
func uintValueOr(value any, fallback uint64) uint64 {
	switch current := value.(type) {
	case json.Number:
		parsed, err := current.Int64()
		if err == nil && parsed >= 0 {
			return uint64(parsed)
		}
	case float64:
		if current >= 0 && current == float64(uint64(current)) {
			return uint64(current)
		}
	case uint64:
		return current
	case int:
		if current >= 0 {
			return uint64(current)
		}
	}
	return fallback
}
