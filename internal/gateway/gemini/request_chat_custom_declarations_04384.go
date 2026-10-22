package gemini

import (
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

const geminiChatCustomGuidance04384 = "Call this custom/freeform tool with the exact raw tool input in the `input` string field."
const geminiChatPatchGuidance04384 = "Call this custom/freeform tool with the exact Codex apply_patch grammar in the `input` string field. The first line must be `*** Begin Patch`, the last line must be `*** End Patch`, and hunks must use `*** Add File:`, `*** Delete File:`, or `*** Update File:`. For `*** Add File: path`, every new file content line must start with `+`; for example `+hello`, and a blank content line is `+`. Do not pass unified diff headers such as `--- a/...` or `+++ b/...` as the top-level input."

func projectGeminiChatCustomDeclarations04384(tool map[string]any) []any {
	name, _ := tool["name"].(string)
	if strings.TrimSpace(name) == "" {
		return nil
	}
	description, _ := tool["description"].(string)
	if strings.TrimSpace(description) == "" {
		description = "Freeform custom tool input."
	}
	guidance, inputDescription := geminiChatCustomGuidance04384, "Exact raw input for the custom/freeform tool."
	if name == "apply_patch" {
		guidance, inputDescription = geminiChatPatchGuidance04384, "Exact raw apply_patch input. For Add File, prefix every new file content line with '+'."
	}
	description += "\n\n" + guidance
	if format, present := tool["format"]; present {
		encoded, err := jsonutil.MarshalSerdeValue04384(format)
		if err != nil {
			return nil
		}
		description += "\n\nOriginal custom tool format JSON: " + string(encoded)
	}
	return []any{map[string]any{"type": "function", "function": map[string]any{
		"name": name, "description": description,
		"parameters": map[string]any{
			"type": "object", "additionalProperties": false, "required": []any{"input"},
			"properties": map[string]any{"input": map[string]any{"type": "string", "description": inputDescription}},
		},
	}}}
}

func validGeminiChatCustomFields04384(tool map[string]any) bool {
	if tool["strict"] == true {
		return false
	}
	if value, present := tool["format"]; present {
		format, ok := value.(map[string]any)
		if !ok {
			return false
		}
		if value, present := format["type"]; present {
			if _, ok := value.(string); !ok {
				return false
			}
		}
	}
	return true
}
