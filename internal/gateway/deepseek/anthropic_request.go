package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

var anthropicChatFields = map[string]bool{
	"model": true, "messages": true, "max_tokens": true, "stream": true,
	"temperature": true, "top_p": true, "stop": true, "tools": true,
	"tool_choice": true, "stream_options": true, "web_search_options": true,
	"parallel_tool_calls": true,
}

func deepSeekAnthropicRequest(body []byte) ([]byte, error) {
	chat, err := parseAnthropicChatRequest(body)
	if err != nil {
		return nil, err
	}
	result, err := anthropicRequestBase(chat)
	if err != nil {
		return nil, err
	}
	if err := applyAnthropicRequestTools(result, chat); err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func parseAnthropicChatRequest(body []byte) (map[string]any, error) {
	var chat map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&chat); err != nil || chat == nil {
		return nil, errors.New("DeepSeek native Messages request must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("DeepSeek native Messages request must be a JSON object")
	}
	fields := make([]string, 0, len(chat))
	for field := range chat {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		if !anthropicChatFields[field] {
			return nil, fmt.Errorf("Anthropic Messages does not translate chat field `%s`", field)
		}
	}
	if parallel, found := chat["parallel_tool_calls"]; found && parallel != true {
		return nil, errors.New("Anthropic Messages only accepts `parallel_tool_calls=true`")
	}
	return chat, nil
}

func anthropicRequestBase(chat map[string]any) (map[string]any, error) {
	messages, system, err := anthropicChatMessages(chat["messages"])
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"max_tokens": 4096, "messages": messages, "model": "auto", "stream": boolField(chat, "stream"),
	}
	if value, found := chat["model"].(string); found {
		result["model"] = value
	}
	if value, found := chat["max_tokens"]; found {
		result["max_tokens"] = value
	}
	if system != "" {
		result["system"] = system
	}
	for _, field := range []string{"temperature", "top_p"} {
		if value, found := chat[field]; found {
			result[field] = value
		}
	}
	if err := applyAnthropicStop(result, chat["stop"]); err != nil {
		return nil, err
	}
	return result, nil
}

func applyAnthropicStop(result map[string]any, stop any) error {
	if stop == nil {
		return nil
	}
	switch stop.(type) {
	case string:
		result["stop_sequences"] = []any{stop}
	case []any:
		result["stop_sequences"] = stop
	default:
		return errors.New("Responses `stop` must be a string or array")
	}
	return nil
}

func applyAnthropicRequestTools(result, chat map[string]any) error {
	search, err := anthropicWebSearchTool(chat["web_search_options"])
	if err != nil {
		return err
	}
	tools, err := anthropicFunctionTools(chat["tools"])
	if err != nil {
		return err
	}
	if choice, found := chat["tool_choice"]; found && choice != "none" {
		translatedChoice, err := anthropicToolChoice(choice)
		if err != nil {
			return err
		}
		result["tool_choice"] = translatedChoice
	}
	if chat["tool_choice"] == "none" {
		return nil
	}
	if search != nil {
		tools = append(tools, search)
	}
	if len(tools) > 0 {
		result["tools"] = tools
	}
	return nil
}

func anthropicToolUse(raw any) (map[string]any, error) {
	call, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("function call must contain function")
	}
	function, ok := call["function"].(map[string]any)
	if !ok {
		return nil, errors.New("function call must contain function")
	}
	name, ok := function["name"].(string)
	if !ok || name == "" {
		return nil, errors.New("function call must contain name")
	}
	name = anthropicToolName(name, stringOr(function["namespace"], ""))
	callID := stringOr(call["id"], "call_prodex")
	arguments := stringOr(function["arguments"], "{}")
	var input map[string]any
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return nil, errors.New("function call arguments must be valid JSON")
	}
	if input == nil {
		return nil, errors.New("function call arguments must be a JSON object")
	}
	return map[string]any{"type": "tool_use", "id": callID, "name": name, "input": input}, nil
}

func anthropicFunctionTools(value any) ([]any, error) {
	items, ok := value.([]any)
	if !ok {
		if value == nil {
			return nil, nil
		}
		return nil, errors.New("Responses `tools` must be an array")
	}
	result := make([]any, 0, len(items))
	for _, raw := range items {
		tool, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("Responses function tool must be an object")
		}
		function, nested := tool["function"].(map[string]any)
		if !nested {
			function = tool
		}
		name, ok := function["name"].(string)
		if !ok || name == "" {
			return nil, errors.New("Responses function tool must contain name")
		}
		name = anthropicToolName(name, stringOr(function["namespace"], ""))
		declaration := map[string]any{"name": name, "input_schema": function["parameters"]}
		if declaration["input_schema"] == nil {
			declaration["input_schema"] = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		if description, found := function["description"]; found {
			declaration["description"] = description
		}
		result = append(result, declaration)
	}
	return result, nil
}

func anthropicWebSearchTool(value any) (map[string]any, error) {
	options, ok := value.(map[string]any)
	if !ok {
		if value == nil {
			return nil, nil
		}
		return nil, errors.New("web_search_options must be an object")
	}
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		switch key {
		case "search_context_size", "allowed_domains", "blocked_domains", "user_location", "max_uses":
		default:
			return nil, fmt.Errorf("Anthropic Messages does not translate web_search_options field `%s`", key)
		}
	}
	if _, allowed := options["allowed_domains"]; allowed {
		if _, blocked := options["blocked_domains"]; blocked {
			return nil, errors.New("Anthropic web search cannot combine allowed_domains and blocked_domains")
		}
	}
	tool := map[string]any{"type": "web_search_20250305", "name": "web_search"}
	for _, field := range []string{"allowed_domains", "blocked_domains", "user_location", "max_uses"} {
		if value, found := options[field]; found {
			tool[field] = value
		}
	}
	if _, found := options["search_context_size"]; found {
		return tool, nil // Anthropic uses its provider-default search context.
	}
	return tool, nil
}

func anthropicToolChoice(value any) (map[string]any, error) {
	switch choice := value.(type) {
	case string:
		switch choice {
		case "auto":
			return map[string]any{"type": "auto"}, nil
		case "required":
			return map[string]any{"type": "any"}, nil
		case "none":
			return nil, nil
		default:
			return nil, fmt.Errorf("unsupported Responses tool_choice `%s`", choice)
		}
	case map[string]any:
		function, _ := choice["function"].(map[string]any)
		name := stringOr(choice["name"], stringOr(function["name"], ""))
		if choice["type"] != "function" || name == "" {
			return nil, errors.New("function tool_choice must contain name")
		}
		return map[string]any{"type": "tool", "name": anthropicToolName(name, stringOr(choice["namespace"], ""))}, nil
	default:
		return nil, errors.New("unsupported Responses tool_choice shape")
	}
}

func anthropicToolName(name, namespace string) string {
	if namespace != "" {
		return namespace + "--" + name
	}
	if dot := strings.LastIndexByte(name, '.'); dot > 0 && dot < len(name)-1 {
		return name[:dot] + "--" + name[dot+1:]
	}
	return name
}

func stringOr(value any, fallback string) string {
	if text, ok := value.(string); ok {
		return text
	}
	return fallback
}

func deepSeekNativeFallbackSafe(err error) bool {
	if err == nil {
		return false
	}
	reason := err.Error()
	return strings.HasPrefix(reason, "Anthropic Messages does not translate chat field ") ||
		strings.HasPrefix(reason, "Anthropic Messages does not translate web_search_options field ") ||
		strings.HasPrefix(reason, "Anthropic web search ")
}

func deepSeekChatFallbackBody(body []byte) ([]byte, error) {
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return nil, errors.New("DeepSeek Responses translation did not produce a JSON object")
	}
	delete(object, "web_search_options")
	return json.Marshal(object)
}
