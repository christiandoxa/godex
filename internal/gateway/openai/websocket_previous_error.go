package openai

import (
	"encoding/json"
	"strings"
)

const (
	websocketPreviousErrorScanLimit     = 2048
	websocketPreviousErrorFieldMaxBytes = 65536
)

func websocketInvalidPreviousResponseID(payload []byte) bool {
	var root any
	if json.Unmarshal(payload, &root) != nil {
		return false
	}
	stack := []any{root}
	visited := 0
	for len(stack) > 0 && visited < websocketPreviousErrorScanLimit {
		index := len(stack) - 1
		value := stack[index]
		stack = stack[:index]
		visited++
		switch typed := value.(type) {
		case map[string]any:
			if websocketInvalidPreviousCandidate(typed) {
				return true
			}
			for _, child := range typed {
				stack = append(stack, child)
			}
		case []any:
			stack = append(stack, typed...)
		}
	}
	return false
}

func websocketInvalidPreviousCandidate(value map[string]any) bool {
	if strings.EqualFold(
		websocketBoundedJSONText(value["code"]),
		"previous_response_not_found",
	) {
		return false
	}
	if !strings.EqualFold(
		websocketBoundedJSONText(value["type"]),
		"invalid_request_error",
	) {
		return false
	}
	message := ""
	for _, key := range []string{"message", "detail", "error"} {
		if candidate := websocketBoundedJSONText(value[key]); candidate != "" {
			message = candidate
			break
		}
	}
	return strings.EqualFold(
		strings.TrimSpace(message),
		"invalid `previous_response_id`.",
	)
}

func websocketBoundedJSONText(value any) string {
	text, ok := value.(string)
	if !ok || len(text) > websocketPreviousErrorFieldMaxBytes {
		return ""
	}
	return strings.TrimSpace(text)
}
