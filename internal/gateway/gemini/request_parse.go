package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

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
	return request, nil
}
