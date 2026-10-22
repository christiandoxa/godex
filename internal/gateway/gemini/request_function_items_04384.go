package gemini

import "github.com/christiandoxa/godex/internal/helper/jsonutil"

func projectGeminiChatFunctionItem04384(kind any, item map[string]any) bool {
	switch kind {
	case "function_call", "mcp_call":
		return projectGeminiChatFunctionCall04384(item)
	case "custom_tool_call":
		return projectGeminiChatCustomCall04384(item)
	case "function_call_output", "custom_tool_call_output", "mcp_tool_result", "mcp_call_output":
		item["type"] = "function_call_output"
		output, _ := firstPresent(item, "output", "content", "result", "error")
		text, err := geminiChatContentText04384(output)
		item["call_id"], item["output"] = geminiChatWireCallIdentity04384(item), text
		return err == nil
	default:
		return true
	}
}

func projectGeminiChatCustomCall04384(item map[string]any) bool {
	name, _ := firstPresent(item, "name", "tool_name")
	textName, _ := name.(string)
	if textName == "" {
		item["type"], item["role"], item["content"] = "message", "assistant", ""
		return true
	}
	input, present := item["input"]
	textInput := ""
	if present {
		var ok bool
		textInput, ok = input.(string)
		if !ok {
			encoded, err := jsonutil.MarshalSerdeValue04384(input)
			if err != nil {
				return false
			}
			textInput = string(encoded)
		}
	}
	arguments, err := jsonutil.MarshalSerdeValue04384(map[string]any{"input": textInput})
	if err != nil {
		return false
	}
	item["type"], item["name"], item["arguments"] = "function_call", textName, string(arguments)
	return projectGeminiChatFunctionCall04384(item)
}

func projectGeminiChatFunctionCall04384(item map[string]any) bool {
	function, _ := item["function"].(map[string]any)
	name, present := firstPresent(item, "name", "tool_name")
	if !present {
		name = function["name"]
	}
	textName, _ := name.(string)
	if textName == "" {
		item["type"], item["role"], item["content"] = "message", "assistant", ""
		return true
	}
	arguments, valid := geminiChatFunctionArguments04384(item)
	item["call_id"], item["name"], item["arguments"] = geminiChatWireCallIdentity04384(item), textName, arguments
	delete(item, "namespace")
	return valid
}

func geminiChatFunctionArguments04384(item map[string]any) (string, bool) {
	function, _ := item["function"].(map[string]any)
	arguments, present := firstPresent(item, "arguments", "input")
	if !present {
		arguments, present = function["arguments"]
	}
	textArguments := "{}"
	if present {
		var ok bool
		textArguments, ok = arguments.(string)
		if !ok {
			encoded, err := jsonutil.MarshalSerdeValue04384(arguments)
			if err != nil {
				return "", false
			}
			textArguments = string(encoded)
		}
	}
	return textArguments, true
}

func geminiChatWireCallIdentity04384(item map[string]any) string {
	value, _ := firstPresent(item, "call_id", "tool_call_id", "id")
	if identity, ok := value.(string); ok {
		return identity
	}
	return "call_1"
}
