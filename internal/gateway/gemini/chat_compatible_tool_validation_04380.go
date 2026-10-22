package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
)

// The 0.438.0 API-key Gemini Chat-compatible bridge validates declarations
// before an upstream request. Flat OpenAI tools are projected to the nested
// function shape *for validation only*, matching the tagged provider's
// tools[index].function.* error paths. The separate Chat declaration projector
// owns the outbound shape.
// Native generateContent/OAuth validation remains independently owned.
func validateGeminiChatCompatibleTools04380(body []byte) error {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		return errors.New("invalid Gemini chat-compatible request")
	}
	raw, exists := request["tools"]
	if !exists {
		return nil
	}
	var decoded any
	if json.Unmarshal(raw, &decoded) != nil {
		return errors.New("invalid_tool_declaration: Gemini request field `tools` must be an array")
	}
	tools, ok := decoded.([]any)
	if !ok {
		return errors.New("invalid_tool_declaration: Gemini request field `tools` must be an array")
	}
	for index, value := range tools {
		tool, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid_tool_declaration: Gemini request field `tools[%d]` must be an object", index)
		}
		kind, _ := tool["type"].(string)
		_, nested := tool["function"]
		if kind == "function" && !nested {
			function := make(map[string]any, 3)
			for _, key := range []string{"name", "parameters", "description"} {
				if value, present := tool[key]; present {
					function[key] = value
				}
			}
			tool = map[string]any{"type": "function", "function": function}
		}
		if err := validateGeminiToolDeclaration(tool, index); err != nil {
			return err
		}
	}
	return nil
}
