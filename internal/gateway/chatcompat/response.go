package chatcompat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	chatMessageKey   = "message"
	chatContentKey   = "content"
	chatCallIDKey    = "call_id"
	chatArgumentsKey = "arguments"
)

type ResponseOptions struct {
	ProviderKey        string
	AdapterLabel       string
	DefaultModel       string
	FallbackResponseID func() string
	FallbackCallID     func(int) string
}

func ChatResponse(body []byte, now time.Time) ([]byte, error) {
	return ChatResponseWithOptions(body, now, ResponseOptions{
		AdapterLabel:       "chat-compatible",
		DefaultModel:       "unknown",
		FallbackResponseID: func() string { return "resp_prodex" },
		FallbackCallID:     func(index int) string { return fmt.Sprintf("call_%d", index) },
	})
}

func ChatResponseWithOptions(body []byte, now time.Time, options ResponseOptions) ([]byte, error) {
	root, err := decodeChatResponse(body)
	if err != nil {
		return nil, err
	}
	options = normalizeResponseOptions(options)
	choice := firstChoice(root)
	message, _ := choice[chatMessageKey].(map[string]any)
	output, toolErr := responseOutput(message, options)
	result := map[string]any{
		"id":         stringOr(root["id"], options.FallbackResponseID()),
		"object":     "response",
		"created_at": uintValueOr(root["created"], uint64(now.Unix())),
		"model":      stringOr(root["model"], options.DefaultModel),
		"output":     output,
	}
	if toolErr != nil {
		result["status"] = "failed"
		result["error"] = map[string]any{
			"code":         "invalid_tool_call_arguments",
			chatMessageKey: toolErr.Error(),
		}
	}
	if usage, present := root["usage"]; present {
		result["usage"] = ResponsesUsage(usage, options.ProviderKey)
	}
	if metadata := chatResponseMetadata(root, choice, message); len(metadata) > 0 && options.ProviderKey != "" {
		result["metadata"] = map[string]any{options.ProviderKey: metadata}
	}
	return json.Marshal(result)
}

func decodeChatResponse(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, errors.New("failed to parse chat completions response JSON")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("failed to parse chat completions response JSON")
	}
	return root, nil
}

func normalizeResponseOptions(options ResponseOptions) ResponseOptions {
	if strings.TrimSpace(options.AdapterLabel) == "" {
		options.AdapterLabel = "chat-compatible"
	}
	if strings.TrimSpace(options.DefaultModel) == "" {
		options.DefaultModel = "unknown"
	}
	if options.FallbackResponseID == nil {
		options.FallbackResponseID = func() string { return "resp_prodex" }
	}
	if options.FallbackCallID == nil {
		options.FallbackCallID = func(index int) string { return fmt.Sprintf("call_%d", index) }
	}
	return options
}

func firstChoice(root map[string]any) map[string]any {
	choices, ok := root["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	return choice
}

func responseOutput(message map[string]any, options ResponseOptions) ([]any, error) {
	output := make([]any, 0)
	output = append(output, responseTextItems(message)...)
	calls, ok := message["tool_calls"].([]any)
	if !ok {
		return output, nil
	}
	for index, raw := range calls {
		item, err := responseToolItem(raw, index, options)
		if err != nil {
			return output, err
		}
		if item != nil {
			output = append(output, item)
		}
	}
	return output, nil
}

func responseTextItems(message map[string]any) []any {
	texts := chatContentTexts(message[chatContentKey])
	if len(texts) == 0 {
		return nil
	}
	parts := make([]any, 0, len(texts))
	for _, text := range texts {
		parts = append(parts, map[string]any{"type": "output_text", "text": text})
	}
	return []any{map[string]any{"type": chatMessageKey, "role": "assistant", chatContentKey: parts}}
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
	text, _ := part[chatContentKey].(string)
	return text
}

func stringOr(value any, fallback string) string {
	text, ok := value.(string)
	if !ok {
		return fallback
	}
	return text
}

func uintValueOr(value any, fallback uint64) uint64 {
	if parsed, ok := uintValue(value); ok {
		return parsed
	}
	return fallback
}

func uintValue(value any) (uint64, bool) {
	switch current := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseUint(current.String(), 10, 64)
		return parsed, err == nil
	case float64:
		if current >= 0 && current < 0x1p64 && current == float64(uint64(current)) {
			return uint64(current), true
		}
	case uint64:
		return current, true
	case int:
		if current >= 0 {
			return uint64(current), true
		}
	}
	return 0, false
}
