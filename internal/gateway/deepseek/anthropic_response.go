package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func deepSeekAnthropicResponse(body []byte, now time.Time) ([]byte, error) {
	var source map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&source); err != nil || source == nil {
		return nil, errors.New("failed to parse native DeepSeek Messages response JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("failed to parse native DeepSeek Messages response JSON")
	}
	content, ok := source["content"].([]any)
	if !ok {
		return nil, errors.New("native DeepSeek Messages response must contain a content array")
	}
	output, err := anthropicResponseOutput(content)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"id": stringOr(source["id"], "resp_anthropic"), "object": "response",
		"created_at": now.Unix(), "model": stringOr(source["model"], "unknown"), "output": output,
	}
	if usage, ok := source["usage"].(map[string]any); ok {
		inputTokens, outputTokens := integerValue(usage["input_tokens"]), integerValue(usage["output_tokens"])
		totalTokens := inputTokens + outputTokens
		if totalTokens < inputTokens {
			totalTokens = ^uint64(0)
		}
		result["usage"] = map[string]any{
			"input_tokens": inputTokens, "output_tokens": outputTokens, "total_tokens": totalTokens,
		}
		if serverUse, ok := usage["server_tool_use"].(map[string]any); ok {
			if searches, ok := serverUse["web_search_requests"]; ok {
				result["tool_usage"] = map[string]any{"web_search": map[string]any{"num_requests": searches}}
			}
		}
	}
	if stopReason, found := source["stop_reason"]; found {
		result["metadata"] = map[string]any{"anthropic": map[string]any{"stop_reason": stopReason}}
	}
	return json.Marshal(result)
}

func anthropicResponseOutput(content []any) ([]any, error) {
	output := make([]any, 0, len(content))
	var textBlocks []any
	flushText := func() {
		if len(textBlocks) > 0 {
			output = append(output, map[string]any{"type": "message", "role": "assistant", "content": textBlocks})
			textBlocks = nil
		}
	}
	for _, raw := range content {
		block, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("Anthropic Messages content block requires type")
		}
		kind, ok := block["type"].(string)
		if !ok {
			return nil, errors.New("Anthropic Messages content block requires type")
		}
		switch kind {
		case "text":
			text, ok := block["text"].(string)
			if !ok {
				return nil, errors.New("Anthropic text block must contain text")
			}
			textBlocks = append(textBlocks, map[string]any{"type": "output_text", "text": text})
		case "thinking":
			if text, ok := block["thinking"].(string); ok {
				flushText()
				output = append(output, map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": text}}})
			}
		case "tool_use":
			flushText()
			item, err := anthropicResponseToolUse(block)
			if err != nil {
				return nil, err
			}
			output = append(output, item)
		case "server_tool_use":
			flushText()
			item, err := anthropicResponseWebSearch(block)
			if err != nil {
				return nil, err
			}
			output = append(output, item)
		case "web_search_tool_result":
			if err := anthropicMergeWebSearchSources(output, block); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported Anthropic Messages content block `%s`", kind)
		}
	}
	flushText()
	return output, nil
}

func anthropicResponseToolUse(block map[string]any) (map[string]any, error) {
	id, ok := block["id"].(string)
	if !ok {
		return nil, errors.New("Anthropic tool_use block must contain id")
	}
	fullName, ok := block["name"].(string)
	if !ok {
		return nil, errors.New("Anthropic tool_use block must contain name")
	}
	input := block["input"]
	if input == nil {
		input = map[string]any{}
	}
	arguments, err := json.Marshal(input)
	if err != nil {
		return nil, errors.New("Anthropic tool input could not be serialized")
	}
	name, shortName := anthropicResponseToolName(fullName)
	item := map[string]any{
		"type": "function_call", "call_id": id, "name": shortName,
		"arguments": chatcompat.WrapRTKArguments(fullName, string(arguments)),
	}
	if name != "" {
		item["namespace"] = name
	}
	return item, nil
}

func anthropicResponseToolName(name string) (string, string) {
	if index := strings.LastIndex(name, "--"); index > 0 && index+2 < len(name) {
		return strings.TrimSpace(name[:index]), strings.TrimSpace(name[index+2:])
	}
	return deepSeekSplitToolName(name)
}

func anthropicResponseWebSearch(block map[string]any) (map[string]any, error) {
	id, ok := block["id"].(string)
	if !ok {
		return nil, errors.New("Anthropic server_tool_use block must contain id")
	}
	if block["name"] != "web_search" {
		return nil, errors.New("unsupported Anthropic server tool")
	}
	input, _ := block["input"].(map[string]any)
	queries := anthropicResponseQueries(input)
	return map[string]any{
		"type": "web_search_call", "id": id, "status": "completed",
		"action": map[string]any{"type": "search", "queries": queries, "sources": []any{}},
	}, nil
}

func anthropicResponseQueries(input map[string]any) []any {
	if query, ok := input["query"].(string); ok {
		return []any{query}
	}
	queries, _ := input["queries"].([]any)
	result := make([]any, 0, len(queries))
	for _, query := range queries {
		if _, ok := query.(string); ok {
			result = append(result, query)
		}
	}
	return result
}

func anthropicMergeWebSearchSources(output []any, block map[string]any) error {
	callID, ok := block["tool_use_id"].(string)
	if !ok {
		return nil
	}
	index := -1
	for i, item := range output {
		call, _ := item.(map[string]any)
		if call["type"] == "web_search_call" && call["id"] == callID {
			index = i
		}
	}
	if index < 0 {
		return nil
	}
	call := output[index].(map[string]any)
	action := call["action"].(map[string]any)
	action["sources"] = anthropicWebSearchSources(block)
	return nil
}

func anthropicWebSearchSources(block map[string]any) []any {
	items, _ := block["content"].([]any)
	result := make([]any, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		url, ok := item["url"].(string)
		if !ok {
			continue
		}
		source := map[string]any{"type": "url", "url": url}
		if title, ok := item["title"].(string); ok {
			source["title"] = title
		}
		result = append(result, source)
	}
	return result
}

func integerValue(value any) uint64 {
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseUint(number.String(), 10, 64)
		if err == nil {
			return parsed
		}
	case float64:
		if number > 0 {
			return uint64(number)
		}
	case uint64:
		return number
	case int:
		if number > 0 {
			return uint64(number)
		}
	}
	return 0
}

func mergeAnthropicResponseMetadata(body []byte, metadata map[string]any) ([]byte, error) {
	if len(metadata) == 0 {
		return body, nil
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil || response == nil {
		return nil, errors.New("failed to parse translated native DeepSeek response JSON")
	}
	mergeResponseMetadata(response, metadata)
	translated, err := json.Marshal(response)
	if err != nil {
		return nil, errors.New("failed to serialize translated native DeepSeek response JSON")
	}
	return translated, nil
}

func nativeMessagesFallbackAllowed(mode string) bool {
	return mode == "" || mode == "auto"
}

func nativeMessagesMode(mode string) bool {
	return mode == "" || mode == "auto" || mode == "anthropic"
}

func nativeMessagesContext(body []byte) bool {
	var request map[string]any
	if json.Unmarshal(body, &request) != nil {
		return false
	}
	_, found := request["web_search_options"]
	return found
}
