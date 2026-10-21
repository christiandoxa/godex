package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func parseResponsesRequest(body []byte) (map[string]any, error) {
	if len(body) > requestMaxBytes {
		return nil, fmt.Errorf("Gemini Responses request exceeds %d bytes", requestMaxBytes)
	}
	// Tagged 0.438.4 gemini_parse_request deserializes the COMPLETE
	// input body through serde_json::Value before looking at model,
	// messages, tools, or generated content. Reject ignored fields whose
	// malformed Unicode/numerics Go's permissive JSON decoder would accept.
	if !geminiSerdeJSONValid04384(body) {
		return nil, errors.New("failed to parse Gemini Responses request JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini Responses request JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("Gemini Responses request body must contain exactly one JSON value")
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
