package main

import (
	"encoding/json"
	"strings"
)

func bufferedFragmentCases04384() []string {
	return []string{"deepseek-buffered-view-literal", "deepseek-buffered-view-overflow", "deepseek-buffered-view-escape", "deepseek-buffered-view-surrogate", "deepseek-buffered-view-depth-limit", "deepseek-buffered-view-depth-empty", "deepseek-buffered-view-depth-over", "deepseek-buffered-view-early-surrogate", "deepseek-buffered-view-early-range", "deepseek-buffered-view-patch"}
}

func bufferedFragmentFixture04384(phase string) (name, arguments, diagnostic string) {
	deep := strings.Repeat("[", 257) + "0" + strings.Repeat("]", 257)
	switch phase {
	case "deepseek-buffered-view-literal":
		return "lookup", "truex", ""
	case "deepseek-buffered-view-overflow":
		return "lookup", "1e309", ""
	case "deepseek-buffered-view-escape":
		return "lookup", `"\q"`, ""
	case "deepseek-buffered-view-surrogate":
		return "lookup", `"\uD800"`, ""
	case "deepseek-buffered-view-depth-limit":
		return "lookup", strings.Repeat("[", 256) + "0" + strings.Repeat("]", 256), ""
	case "deepseek-buffered-view-depth-empty":
		return "lookup", strings.Repeat("[", 257) + strings.Repeat("]", 257), ""
	case "deepseek-buffered-view-depth-over":
		return "lookup", deep, "recursion limit exceeded at line 1 column 128"
	case "deepseek-buffered-view-early-surrogate":
		return "lookup", `{"v":"\uD800","deep":` + deep + `}`, "unexpected end of hex escape at line 1 column 13"
	case "deepseek-buffered-view-early-range":
		return "lookup", `{"v":1e309,"deep":` + deep + `}`, "number out of range at line 1 column 10"
	case "deepseek-buffered-view-patch":
		return "apply_patch", "truex", ""
	}
	return "", "", ""
}

func bufferedFragmentResponse04384(phase string) string {
	name, arguments, diagnostic := bufferedFragmentFixture04384(phase)
	if diagnostic != "" {
		return bufferedToolFailedResponse04384(name, diagnostic)
	}
	if name == "" {
		return ""
	}
	item := map[string]any{"type": "function_call", "call_id": "call-differential", "name": name, "arguments": arguments}
	if name == "apply_patch" {
		item["type"], item["input"] = "custom_tool_call", arguments
		delete(item, "arguments")
	}
	output, _ := json.Marshal([]any{item})
	return `{"created_at":1,"id":"chatcmpl-differential-tool","metadata":{"deepseek":{"finish_reason":"tool_calls"}},"model":"deepseek-v4-pro","object":"response","output":` + string(output) + `,"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
}
