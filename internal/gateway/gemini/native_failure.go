package gemini

import (
	"encoding/json"
	"strconv"
)

func geminiNativeStreamError(root map[string]any) (string, string, bool) {
	value, exists := root["error"]
	if !exists {
		return "", "", false
	}
	failure, _ := value.(map[string]any)
	status, exists := failure["status"]
	if !exists {
		status = failure["code"]
	}
	code := "provider_stream_error"
	switch value := status.(type) {
	case string:
		code = value
	case json.Number:
		// Serde decodes negative zero as a float, so as_i64 rejects it.
		if integer, err := value.Int64(); err == nil && value != "-0" {
			code = strconv.FormatInt(integer, 10)
		}
	}
	message, valid := failure["message"].(string)
	if !valid {
		message = "Gemini stream returned an embedded provider error"
	}
	return code, message, true
}

func geminiNativePromptFailure(root map[string]any) (string, bool) {
	feedback, _ := root["promptFeedback"].(map[string]any)
	reason := nativeNonblank(feedback["blockReason"])
	if reason == "" {
		return "", false
	}
	return "Gemini blocked the prompt: " + reason, true
}
