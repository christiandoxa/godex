package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
		if geminiChatToolHasLocalMedia04384(tool, 0) {
			continue
		}
		kind, _ := tool["type"].(string)
		_, nested := tool["function"]
		if nested {
			if err := validateGeminiToolDeclaration(map[string]any{"function": tool["function"]}, index); err != nil {
				return err
			}
			continue
		}
		if kind != "function" && geminiChatBuiltinTool04384(tool) {
			continue
		}
		if kind != "function" {
			var converted []any
			if kind == "custom" {
				converted = projectGeminiChatCustomDeclarations04384(tool)
			} else if kind == "namespace" {
				converted = projectGeminiChatNamespaceDeclarations04384(tool)
			} else if kind == "tool_search" {
				converted = projectGeminiChatSearchDeclarations04384(tool)
			} else if strings.HasPrefix(kind, "mcp") {
				converted = projectGeminiChatMCPDeclarations04384(tool)
			} else {
				converted = projectGeminiChatMCPAlias04384(tool)
			}
			if len(converted) == 0 {
				return fmt.Errorf("invalid_tool_declaration: Gemini request field `tools[%d]` is not a supported tool declaration", index)
			}
			for translatedIndex, raw := range converted {
				if err := validateGeminiToolDeclaration(raw, translatedIndex); err != nil {
					return fmt.Errorf("invalid_tool_declaration: Gemini request field `tools[%d]` translates to an invalid declaration: %w", index, err)
				}
			}
			continue
		}
		if kind == "function" {
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

func geminiChatToolHasLocalMedia04384(value any, depth int) bool {
	if depth > 256 {
		return false
	}
	switch value := value.(type) {
	case map[string]any:
		switch value["type"] {
		case "input_image", "image_url", "input_file", "file", "media", "input_audio", "input_video":
			for _, key := range []string{"path", "file_path", "filePath"} {
				if _, present := value[key]; present {
					return true
				}
			}
		}
		for _, nested := range value {
			if geminiChatToolHasLocalMedia04384(nested, depth+1) {
				return true
			}
		}
	case []any:
		for _, nested := range value {
			if geminiChatToolHasLocalMedia04384(nested, depth+1) {
				return true
			}
		}
	}
	return false
}
