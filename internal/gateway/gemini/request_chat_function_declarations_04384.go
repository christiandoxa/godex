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
		kind, _ := tool["type"].(string)
		if kind != "function" && kind != "custom" && kind != "namespace" && kind != "tool_search" && !strings.HasPrefix(kind, "mcp") && geminiChatBuiltinTool04384(tool) {
			converted, valid := geminiChatBuiltinFunction04384(tool)
			if !valid {
				return false
			}
			if converted != nil {
				tools = append(tools, converted)
			}
			continue
		}
		if kind == "custom" || kind == "namespace" || kind == "tool_search" || strings.HasPrefix(kind, "mcp") {
			function, _ := tool["function"].(map[string]any)
			if !validGeminiChatFunctionFields04384(tool) || !validGeminiChatFunctionFields04384(function) {
				return false
			}
			var projected []any
			if kind == "custom" {
				if !validGeminiChatCustomFields04384(tool) {
					return false
				}
				projected = projectGeminiChatCustomDeclarations04384(tool)
			} else if kind == "namespace" {
				if !validGeminiChatNamespaceFields04384(tool) {
					return false
				}
				projected = projectGeminiChatNamespaceDeclarations04384(tool)
			} else if kind == "tool_search" {
				projected = projectGeminiChatSearchDeclarations04384(tool)
			} else {
				projected = projectGeminiChatMCPDeclarations04384(tool)
			}
			for _, raw := range projected {
				converted, valid := geminiChatFunctionDeclaration04384(raw.(map[string]any))
				if !valid {
					return false
				}
				tools = append(tools, converted)
			}
			continue
		}
		if tool["type"] == "function" {
			converted, valid := geminiChatFunctionDeclaration04384(tool)
			if !valid {
				return false
			}
			tools = append(tools, converted)
		} else {
			if _, typed := tool["type"].(string); typed {
				return false
			}
			for _, raw := range projectGeminiChatMCPAlias04384(tool) {
				converted, valid := geminiChatFunctionDeclaration04384(raw.(map[string]any))
				if !valid {
					return false
				}
				tools = append(tools, converted)
			}
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
	rawParameters, present := function["parameters"]
	parameters, ok := rawParameters.(map[string]any)
	if !validGeminiChatFunctionName04384(name) || strict || present && !ok {
		return nil, false
	}
	converted := map[string]any{"name": name}
	if present {
		converted["parameters"] = parameters
	}
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
