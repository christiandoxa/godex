package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

const requestMaxBytes = 16 << 20

type translatedRequest struct {
	body     []byte
	metadata map[string]any
	model    string
	stream   bool
}

func translateResponsesRequest(body []byte, model string) (translatedRequest, error) {
	request, err := parseResponsesRequest(body)
	if err != nil {
		return translatedRequest{}, err
	}
	chat, err := translateBaseRequest(request, model)
	if err != nil {
		return translatedRequest{}, err
	}
	metadata, err := applyGeminiRequestControls(chat, request)
	if err != nil {
		return translatedRequest{}, err
	}
	applyThoughtSignatures(chat["messages"], request["input"])
	native, err := geminiGenerateContentRequest(chat, request)
	if err != nil {
		return translatedRequest{}, err
	}
	encoded, err := json.Marshal(native)
	if err != nil {
		return translatedRequest{}, errors.New("failed to serialize translated Gemini generateContent request")
	}
	selectedModel := strings.TrimSpace(model)
	if requested, _ := request["model"].(string); strings.TrimSpace(requested) != "" {
		selectedModel = strings.TrimSpace(requested)
	}
	if selectedModel == "" {
		selectedModel = "auto"
	}
	stream, _ := request["stream"].(bool)
	return translatedRequest{body: encoded, metadata: metadata, model: selectedModel, stream: stream}, nil
}

func parseResponsesRequest(body []byte) (map[string]any, error) {
	if len(body) > requestMaxBytes {
		return nil, fmt.Errorf("Gemini Responses request exceeds %d bytes", requestMaxBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini Responses request JSON: %w", err)
	}
	request, ok := root.(map[string]any)
	if !ok {
		return nil, errors.New("Gemini Responses request body must be a JSON object")
	}
	if value, exists := request["model"]; exists {
		if _, ok := value.(string); !ok {
			return nil, errors.New("Gemini OpenAI-compatible model must be a string")
		}
	}
	if err := validateInputItems(request["input"]); err != nil {
		return nil, err
	}
	return request, nil
}

func translateBaseRequest(request map[string]any, model string) (map[string]any, error) {
	copy := make(map[string]any, len(request))
	for key, value := range request {
		switch key {
		case "reasoning", "reasoning_effort", "response_format", "text", "tools", "tool_choice", "user_id", "stop_sequences", "top_logprobs", "safety_identifier", "web_search_options", "logprobs", "metadata", "client_metadata", "prompt_cache_key", "prompt_cache_retention", "parallel_tool_calls":
			continue
		default:
			copy[key] = value
		}
	}
	encoded, err := json.Marshal(copy)
	if err != nil {
		return nil, errors.New("failed to serialize Gemini Responses request")
	}
	chatBody, err := chatcompat.ResponsesRequest(encoded, "auto", model)
	if err != nil {
		return nil, fmt.Errorf("Gemini OpenAI-compatible request translation failed: %w", err)
	}
	var chat map[string]any
	if err := json.Unmarshal(chatBody, &chat); err != nil {
		return nil, errors.New("failed to parse translated Gemini chat request")
	}
	if parallel, ok := request["parallel_tool_calls"].(bool); ok {
		chat["parallel_tool_calls"] = parallel
	}
	return chat, nil
}

func applyGeminiRequestControls(chat, request map[string]any) (map[string]any, error) {
	metadata := make(map[string]any)
	effort, err := geminiReasoningEffort(request)
	if err != nil {
		return nil, err
	}
	if effort != "" {
		chat["reasoning_effort"] = effort
	}
	if err := applyGeminiToolsAndChoice(chat, request, metadata, effort); err != nil {
		return nil, err
	}
	if err := applyGeminiFormat(chat, request, metadata); err != nil {
		return nil, err
	}
	if err := applyGeminiOtherControls(chat, request); err != nil {
		return nil, err
	}
	return geminiResponseMetadata(request, metadata)
}

func applyGeminiToolsAndChoice(chat, request, metadata map[string]any, effort string) error {
	tools, names, err := geminiTools(request["tools"])
	if err != nil {
		return err
	}
	if len(tools) > 0 {
		chat["tools"] = tools
	}
	if webSearch := geminiWebSearchOptions(request["tools"]); webSearch != nil {
		chat["web_search_options"] = webSearch
	}
	choice, err := geminiToolChoice(request["tool_choice"], names)
	if err != nil {
		return err
	}
	if choice != nil && reasoningEnabled(effort) {
		metadata["omitted_tool_choice"] = map[string]any{
			"from":   request["tool_choice"],
			"reason": "Gemini OpenAI-compatible thinking mode does not accept explicit tool_choice",
		}
	} else if choice != nil {
		chat["tool_choice"] = choice
	}
	return nil
}

func applyGeminiFormat(chat, request, metadata map[string]any) error {
	format, degraded, err := geminiResponseFormat(request)
	if err != nil {
		return err
	}
	if format != nil {
		chat["response_format"] = format
	}
	if degraded {
		// Native Gemini GenerateContent supports JSON-schema constraints directly.
		// The schema is projected into generationConfig.responseJsonSchema later.
	}
	return nil
}

func applyGeminiOtherControls(chat, request map[string]any) error {
	if userID, exists := request["user_id"]; exists {
		value, ok := userID.(string)
		if !ok {
			return errors.New("Gemini OpenAI-compatible user_id must be a string")
		}
		chat["user_id"] = value
	}
	if stop, exists := request["stop_sequences"]; exists {
		chat["stop"] = stop
	}
	if topLogprobs, exists := request["top_logprobs"]; exists {
		chat["top_logprobs"] = topLogprobs
	}
	if stream, _ := chat["stream"].(bool); stream {
		chat["stream_options"] = map[string]any{"include_usage": true}
	}
	return nil
}

func applyThoughtSignatures(messages, input any) {
	signatures := make(map[string]string)
	if items, ok := input.([]any); ok {
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok || item["type"] != "function_call" {
				continue
			}
			signature, present, valid := geminiThoughtSignature(item)
			callID, _ := item["call_id"].(string)
			if present && valid && callID != "" {
				signatures[callID] = signature
			}
		}
	}
	converted, _ := messages.([]any)
	for _, raw := range converted {
		message, _ := raw.(map[string]any)
		calls, _ := message["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			signature, present, valid := geminiThoughtSignature(call)
			if present {
				if !valid {
					continue
				}
			} else {
				callID, _ := call["id"].(string)
				signature = signatures[callID]
				if signature == "" {
					continue
				}
			}
			for _, key := range []string{"gemini_thought_signature", "thought_signature", "thoughtSignature"} {
				delete(call, key)
			}
			call["extra_content"] = map[string]any{"google": map[string]any{"thought_signature": signature}}
		}
	}
}

func geminiThoughtSignature(object map[string]any) (string, bool, bool) {
	for _, path := range [][]string{
		{"extra_content", "google", "thought_signature"},
		{"gemini_thought_signature"},
		{"thought_signature"},
		{"thoughtSignature"},
		{"function", "gemini_thought_signature"},
		{"function", "thought_signature"},
		{"function", "thoughtSignature"},
	} {
		value, present := geminiSignatureField(object, path...)
		if !present {
			continue
		}
		text, ok := value.(string)
		return text, true, ok && strings.TrimSpace(text) != ""
	}
	return "", false, false
}

func geminiSignatureField(object map[string]any, path ...string) (any, bool) {
	var value any = object
	for _, key := range path {
		current, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = current[key]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

func ensureJSONInstruction(chat map[string]any) {
	messages, _ := chat["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		if role == "system" {
			message["content"] = strings.TrimSpace(fmt.Sprint(message["content"]) + "\nReturn valid JSON without markdown fences.")
			return
		}
	}
	chat["messages"] = append([]any{map[string]any{"role": "system", "content": "Return valid JSON without markdown fences."}}, messages...)
}
