package gemini

import "github.com/christiandoxa/godex/internal/helper/jsonutil"

func projectGeminiChatFunctionItem04384(kind any, item map[string]any) bool {
	switch kind {
	case "function_call":
		return projectGeminiChatFunctionCall04384(item)
	case "function_call_output":
		output, _ := firstPresent(item, "output", "content", "result", "error")
		text, err := geminiChatContentText04384(output)
		item["call_id"], item["output"] = geminiChatWireCallIdentity04384(item), text
		return err == nil
	default:
		return true
	}
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
				return false
			}
			textArguments = string(encoded)
		}
	}
	item["call_id"], item["name"], item["arguments"] = geminiChatWireCallIdentity04384(item), textName, textArguments
	delete(item, "namespace")
	return true
}

func geminiChatWireCallIdentity04384(item map[string]any) string {
	value, _ := firstPresent(item, "call_id", "tool_call_id", "id")
	if identity, ok := value.(string); ok {
		return identity
	}
	return "call_1"
}
