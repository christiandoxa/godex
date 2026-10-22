package gemini

import (
	"bytes"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func normalizeGeminiChatFunctionDeclarations04384(root map[string]any) bool {
	value, present := root["tools"]
	if !present {
		return true
	}
	entries, ok := value.([]any)
	if !ok {
		return false
	}
	tools := make([]any, 0, len(entries))
	for _, raw := range entries {
		tool, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		if tool["type"] == "function" {
			converted, valid := geminiChatFunctionDeclaration04384(tool)
			if !valid {
				return false
			}
			tools = append(tools, converted)
		} else {
			// shortcut: non-function declarations keep existing conversion until their wire is qualified.
			tools = append(tools, raw)
		}
	}
	tools, valid := deduplicateGeminiChatFunctions04384(tools)
	if !valid || len(tools) > 128 {
		return false
	}
	if len(tools) == 0 {
		delete(root, "tools")
	} else {
		root["tools"] = tools
	}
	return true
}

func geminiChatFunctionDeclaration04384(tool map[string]any) (map[string]any, bool) {
	function, ok := tool["function"].(map[string]any)
	if !ok {
		function = tool
	}
	if !validGeminiChatFunctionFields04384(tool) || !validGeminiChatFunctionFields04384(function) {
		return nil, false
	}
	name, _ := function["name"].(string)
	strict, _ := function["strict"].(bool)
	parameters, ok := function["parameters"].(map[string]any)
	if !validGeminiChatFunctionName04384(name) || strict || !ok {
		return nil, false
	}
	converted := map[string]any{"name": name, "parameters": parameters}
	if description, _ := function["description"].(string); strings.TrimSpace(description) != "" {
		converted["description"] = description
	}
	if _, present := function["strict"]; present {
		converted["strict"] = false
	}
	return map[string]any{"type": "function", "function": converted}, true
}

func validGeminiChatFunctionFields04384(function map[string]any) bool {
	if value, present := function["description"]; present {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	if value, present := function["strict"]; present {
		if _, ok := value.(bool); !ok {
			return false
		}
	}
	return true
}

func validGeminiChatFunctionName04384(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for _, char := range []byte(name) {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-') {
			return false
		}
	}
	return true
}

func deduplicateGeminiChatFunctions04384(tools []any) ([]any, bool) {
	result := make([]any, 0, len(tools))
	seen := make(map[string]int)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if tool["type"] != "function" {
			result = append(result, raw)
			continue
		}
		function, _ := tool["function"].(map[string]any)
		name, named := function["name"].(string)
		if prior, exists := seen[name]; named && exists {
			if genericGeminiChatFunction04384(tool) {
				continue
			}
			if !genericGeminiChatFunction04384(result[prior].(map[string]any)) {
				if !equalGeminiChatFunctions04384(result[prior], tool) {
					return nil, false
				}
				continue
			}
			result[prior] = nil
		}
		if named {
			seen[name] = len(result)
		}
		result = append(result, raw)
	}
	compacted := result[:0]
	for _, tool := range result {
		if tool != nil {
			compacted = append(compacted, tool)
		}
	}
	return compacted, true
}

func genericGeminiChatFunction04384(tool map[string]any) bool {
	function, _ := tool["function"].(map[string]any)
	parameters, _ := function["parameters"].(map[string]any)
	_, properties := parameters["properties"]
	_, required := parameters["required"]
	return parameters["additionalProperties"] == true && !properties && !required
}

func equalGeminiChatFunctions04384(left, right any) bool {
	a, leftErr := jsonutil.MarshalSerdeValue04384(left)
	b, rightErr := jsonutil.MarshalSerdeValue04384(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	quoted, escaped := false, false
	x, y := 0, 0
	for x < len(a) && y < len(b) {
		if !quoted {
			if geminiChatNegativeZero04384(a, x) {
				x++
			}
			if geminiChatNegativeZero04384(b, y) {
				y++
			}
		}
		char := a[x]
		if char != b[y] {
			return false
		}
		if quoted {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				quoted = false
			}
		} else if char == '"' {
			quoted = true
		}
		x++
		y++
	}
	return x == len(a) && y == len(b)
}

func geminiChatNegativeZero04384(body []byte, index int) bool {
	end := index + len("-0.0")
	return end <= len(body) && bytes.Equal(body[index:end], []byte("-0.0")) &&
		(end == len(body) || body[end] == ',' || body[end] == ']' || body[end] == '}')
}
