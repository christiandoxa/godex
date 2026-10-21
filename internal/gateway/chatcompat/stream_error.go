package chatcompat

import (
	"encoding/json"
	"strconv"
)

// ChatStreamError classifies a present Chat Completions error value.
// Callers preserve numeric kinds with json.Number before classification.
func ChatStreamError(value any) (code, message string) {
	if message, ok := value.(string); ok {
		return "provider_stream_error", message
	}
	failure, _ := value.(map[string]any)
	value, exists := failure["type"]
	if !exists {
		value = failure["code"]
	}
	code = "provider_stream_error"
	switch value := value.(type) {
	case string:
		code = value
	case json.Number:
		if integer, err := value.Int64(); err == nil && value != "-0" {
			code = strconv.FormatInt(integer, 10)
		}
	}
	value, exists = failure["message"]
	if !exists {
		value = failure["detail"]
	}
	message, ok := value.(string)
	if !ok {
		message = "Provider stream returned an embedded error"
	}
	return code, message
}
