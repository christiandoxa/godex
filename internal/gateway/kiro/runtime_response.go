package kiro

import (
	"encoding/json"
	"fmt"
	"strings"
)

func kiroChatResponse(response map[string]any, requestID uint64) map[string]any {
	id := "chatcmpl_kiro_" + fmt.Sprintf("%d", requestID)
	if rawID, ok := response["id"].(string); ok && rawID != "" {
		if strings.HasPrefix(rawID, "resp_") {
			id = "chatcmpl_" + strings.TrimPrefix(rawID, "resp_")
		} else {
			id = "chatcmpl_" + rawID
		}
	}
	message := map[string]any{"role": kiroRoleAssistant, kiroFieldContent: kiroResponseText(response)}
	if reasoning := kiroResponseReasoning(response); reasoning != "" {
		message["reasoning_content"] = reasoning
	}
	if status, _ := response["status"].(string); status == "failed" {
		if errorValue, found := response["error"]; found && errorValue != nil {
			message["refusal"] = errorMessageValue(errorValue)
		} else {
			message["refusal"] = "Kiro request failed"
		}
	}
	choice := map[string]any{
		"index":          0,
		kiroFieldMessage: message,
		"finish_reason":  kiroChatFinishReason(response),
	}
	result := map[string]any{
		"id":           id,
		"object":       "chat.completion",
		"created":      responseUint(response["created_at"]),
		kiroFieldModel: responseString(response[kiroFieldModel], "kiro-cli"),
		"choices":      []any{choice},
	}
	if value, found := response["requested_model"]; found {
		result["requested_model"] = value
	}
	if value, found := response[kiroFieldMetadata]; found {
		result[kiroFieldMetadata] = value
	}
	if value, found := response[kiroFieldUsage]; found {
		result[kiroFieldUsage] = value
	}
	return result
}

func kiroMessagesResponse(response map[string]any, requestedModel string) map[string]any {
	content := make([]any, 0, 2)
	output, _ := response["output"].([]any)
	for _, raw := range output {
		item, ok := raw.(map[string]any)
		if !ok || responseString(item["type"], "") != "function_call" {
			continue
		}
		id, found := item["call_id"]
		if !found {
			id = "call_kiro"
		}
		name, found := item["name"]
		if !found {
			name = "tool_call"
		}
		input := any(map[string]any{})
		if arguments, ok := item["arguments"].(string); ok {
			var decoded any
			if json.Unmarshal([]byte(arguments), &decoded) == nil {
				input = decoded
			}
		}
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  name,
			"input": input,
		})
	}
	if text := kiroResponseText(response); text != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	usage := map[string]any{kiroFieldInputTokens: 0, kiroFieldOutputTokens: 0}
	if current, ok := response[kiroFieldUsage].(map[string]any); ok {
		if value, found := current[kiroFieldInputTokens]; found {
			usage[kiroFieldInputTokens] = value
		}
		if value, found := current[kiroFieldOutputTokens]; found {
			usage[kiroFieldOutputTokens] = value
		}
	}
	id, found := response["id"]
	if !found {
		id = "msg_kiro"
	}
	return map[string]any{
		"id":             id,
		"type":           kiroFieldMessage,
		"role":           kiroRoleAssistant,
		kiroFieldModel:   requestedModel,
		kiroFieldContent: content,
		"stop_reason":    kiroMessagesStopReason(response),
		"stop_sequence":  nil,
		kiroFieldUsage:   usage,
	}
}

func kiroResponseText(response map[string]any) string {
	output, ok := response["output"].([]any)
	if !ok {
		return ""
	}
	for _, raw := range output {
		if text := kiroOutputMessageText(raw); text != "" {
			return text
		}
	}
	return ""
}

func kiroOutputMessageText(raw any) string {
	item, ok := raw.(map[string]any)
	if !ok || responseString(item["type"], "") != kiroFieldMessage {
		return ""
	}
	content, ok := item[kiroFieldContent].([]any)
	if !ok {
		return ""
	}
	for _, rawPart := range content {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := part["text"].(string); ok {
			return text
		}
	}
	return ""
}

func kiroResponseReasoning(response map[string]any) string {
	metadata, _ := response[kiroFieldMetadata].(map[string]any)
	kiro, _ := metadata["kiro"].(map[string]any)
	text, _ := kiro["reasoning_content"].(string)
	return text
}

func kiroChatFinishReason(response map[string]any) string {
	if kiroResponseHasToolCalls(response) {
		return "tool_calls"
	}
	if details, ok := response["incomplete_details"].(map[string]any); ok {
		if responseString(details["reason"], "") == "max_output_tokens" {
			return "length"
		}
	}
	return "stop"
}

func kiroMessagesStopReason(response map[string]any) string {
	if kiroResponseHasToolCalls(response) {
		return "tool_use"
	}
	var reason any
	var found bool
	if details, ok := response["incomplete_details"].(map[string]any); ok {
		reason, found = details["reason"]
	}
	if !found {
		if metadata, ok := response[kiroFieldMetadata].(map[string]any); ok {
			if kiro, ok := metadata["kiro"].(map[string]any); ok {
				reason, found = kiro["stop_reason"]
			}
		}
	}
	value, _ := reason.(string)
	switch value {
	case "max_output_tokens", "max_tokens":
		return "max_tokens"
	case "tool_use":
		return "tool_use"
	default:
		return "end_turn"
	}
}

func kiroResponseHasToolCalls(response map[string]any) bool {
	output, _ := response["output"].([]any)
	for _, raw := range output {
		if item, ok := raw.(map[string]any); ok && responseString(item["type"], "") == "function_call" {
			return true
		}
	}
	return false
}

func errorMessageValue(value any) any {
	if object, ok := value.(map[string]any); ok {
		if message, found := object[kiroFieldMessage]; found {
			return message
		}
	}
	return "Kiro request failed"
}

func responseString(value any, fallback string) string {
	text, ok := value.(string)
	if !ok || text == "" {
		return fallback
	}
	return text
}

func responseUint(value any) uint64 {
	switch current := value.(type) {
	case float64:
		if current >= 0 {
			return uint64(current)
		}
	case int64:
		if current >= 0 {
			return uint64(current)
		}
	case json.Number:
		if parsed, err := current.Int64(); err == nil && parsed >= 0 {
			return uint64(parsed)
		}
	}
	return 0
}
