package deepseek

import (
	"bytes"
	"encoding/json"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
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
	code, message := chatcompat.ChatStreamError(raw)
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
