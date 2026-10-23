package openai

import (
	"bytes"
	"encoding/json"
	"slices"
)

// On validated JSON, match Serde's exact names and reject repeated known fields.
func knownJSONFields(body []byte, names ...string) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, false
	}
	fields := make(map[string]json.RawMessage, len(names))
	for decoder.More() {
		token, err := decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		if !slices.Contains(names, name) {
			continue
		}
		if _, exists := fields[name]; exists {
			return nil, false
		}
		fields[name] = value
	}
	return fields, true
}
