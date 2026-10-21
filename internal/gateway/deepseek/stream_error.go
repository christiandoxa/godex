package deepseek

import (
	"bytes"
	"encoding/json"
	"strconv"
)

func deepSeekEmbeddedStreamError(root map[string]any, data []byte) (string, string, bool) {
	_, exists := root["error"]
	if !exists {
		return "", "", false
	}
	raw, err := deepSeekStreamErrorValue(data)
	if err != nil {
		return "provider_stream_error", "Provider stream returned an embedded error", true
	}
	if message, ok := raw.(string); ok {
		return "provider_stream_error", message, true
	}
	failure, _ := raw.(map[string]any)
	value, exists := failure["type"]
	if !exists {
		value = failure["code"]
	}
	code := "provider_stream_error"
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
	return code, message, true
}

func deepSeekStreamErrorValue(data []byte) (any, error) {
	// Preserve exact member names and numeric types only on the failure path.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope["error"]))
	decoder.UseNumber()
	var value any
	err := decoder.Decode(&value)
	return value, err
}
