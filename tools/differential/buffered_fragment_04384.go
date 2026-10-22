package main

import (
	"encoding/json"
	"strings"
)

func bufferedFragmentCases04384() []string {
	return []string{
		"deepseek-buffered-view-literal",
		"deepseek-buffered-view-overflow",
		"deepseek-buffered-view-escape",
		"deepseek-buffered-view-surrogate",
		"deepseek-buffered-view-depth-limit",
		"deepseek-buffered-view-depth-empty",
		"deepseek-buffered-view-depth-over",
		"deepseek-buffered-view-early-surrogate",
		"deepseek-buffered-view-early-range",
		"deepseek-buffered-view-patch",
		"deepseek-buffered-view-array-comma",
		"deepseek-buffered-view-array-separator",
		"deepseek-buffered-view-trailing",
		"deepseek-buffered-view-search-array-comma",
		"deepseek-buffered-view-search-array-separator",
		"deepseek-buffered-view-search-trailing",
		"deepseek-buffered-view-eof-list",
		"deepseek-buffered-view-eof-object",
		"deepseek-buffered-view-eof-value",
		"deepseek-buffered-view-eof-array-value",
		"deepseek-buffered-view-search-eof-value",
		"deepseek-buffered-view-eof-early-range",
		"deepseek-buffered-view-eof-early-surrogate",
		"deepseek-buffered-view-search-output-overflow",
		"deepseek-buffered-view-search-output-overflow-line",
		"deepseek-buffered-view-search-output-surrogate",
		"deepseek-buffered-view-search-output-depth-limit",
		"deepseek-buffered-view-search-output-depth-over",
		"deepseek-buffered-view-search-output-early-surrogate",
		"deepseek-buffered-view-search-depth-kernel-reject",
		"deepseek-buffered-view-search-runtime-depth-over",
		"deepseek-buffered-view-search-runtime-depth-over-rotation",
		"deepseek-buffered-view-eof-string",
		"deepseek-buffered-view-eof-key",
		"deepseek-buffered-view-eof-string-unicode",
		"deepseek-buffered-view-eof-string-escape",
		"deepseek-buffered-view-search-eof-string",
		"deepseek-buffered-view-eof-string-surrogate",
		"deepseek-buffered-view-eof-string-surrogate-pair",
		"deepseek-buffered-view-eof-literal-value",
		"deepseek-buffered-view-eof-string-early-range",
		"deepseek-buffered-view-search-output-surrogate-quote",
	}
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
	case "deepseek-buffered-view-array-comma":
		return "lookup", "[1,]", "trailing comma at line 1 column 4"
	case "deepseek-buffered-view-array-separator":
		return "lookup", "[1 2]", "expected `,` or `]` at line 1 column 4"
	case "deepseek-buffered-view-trailing":
		return "lookup", "{} true", "trailing characters at line 1 column 4"
	case "deepseek-buffered-view-search-array-comma":
		return "tool_search", "[1,]", "trailing comma at line 1 column 4"
	case "deepseek-buffered-view-search-array-separator":
		return "tool_search", "[1 2]", "expected `,` or `]` at line 1 column 4"
	case "deepseek-buffered-view-search-trailing":
		return "tool_search", "{} true", "trailing characters at line 1 column 4"
	case "deepseek-buffered-view-eof-list":
		return "lookup", "[1", "EOF while parsing a list at line 1 column 2"
	case "deepseek-buffered-view-eof-object":
		return "lookup", `{"k"`, "EOF while parsing an object at line 1 column 4"
	case "deepseek-buffered-view-eof-value":
		return "lookup", `{"k":`, "EOF while parsing a value at line 1 column 5"
	case "deepseek-buffered-view-eof-array-value":
		return "lookup", "[1,", "EOF while parsing a value at line 1 column 3"
	case "deepseek-buffered-view-search-eof-value":
		return "tool_search", `{"k":`, "EOF while parsing a value at line 1 column 5"
	case "deepseek-buffered-view-eof-early-range":
		return "lookup", `{"v":1e309,"k":`, "number out of range at line 1 column 10"
	case "deepseek-buffered-view-eof-early-surrogate":
		return "lookup", `{"v":"\uD800","k":`, "unexpected end of hex escape at line 1 column 13"
	case "deepseek-buffered-view-search-output-overflow":
		return "tool_search", "1e309", "number out of range at line 1 column 95"
	case "deepseek-buffered-view-search-output-overflow-line":
		return "tool_search", "\n1e309", "number out of range at line 2 column 5"
	case "deepseek-buffered-view-search-output-surrogate":
		return "tool_search", `"\uD800"`, "unexpected end of hex escape at line 1 column 98"
	case "deepseek-buffered-view-search-output-depth-limit":
		return "tool_search", strings.Repeat("[", 124) + "0" + strings.Repeat("]", 124), ""
	case "deepseek-buffered-view-search-output-depth-over":
		return "tool_search", strings.Repeat("[", 127) + "0" + strings.Repeat("]", 127), "recursion limit exceeded at line 1 column 217"
	case "deepseek-buffered-view-search-output-early-surrogate":
		return "tool_search", `{"v":"\uD800","deep":` + strings.Repeat("[", 200) + "0" + strings.Repeat("]", 200) + `}`, "unexpected end of hex escape at line 1 column 103"
	case "deepseek-buffered-view-search-depth-kernel-reject":
		return "tool_search", deep, "recursion limit exceeded at line 1 column 128"
	case "deepseek-buffered-view-search-runtime-depth-over", "deepseek-buffered-view-search-runtime-depth-over-rotation":
		return "tool_search", strings.Repeat("[", 125) + "0" + strings.Repeat("]", 125), ""
	case "deepseek-buffered-view-eof-string":
		return "lookup", `"abc`, "EOF while parsing a string at line 1 column 4"
	case "deepseek-buffered-view-eof-key":
		return "lookup", `{"ke`, "EOF while parsing a string at line 1 column 4"
	case "deepseek-buffered-view-eof-string-unicode":
		return "lookup", "\n\"雪", "EOF while parsing a string at line 2 column 4"
	case "deepseek-buffered-view-eof-string-escape":
		return "lookup", `"abc\`, "EOF while parsing a string at line 1 column 5"
	case "deepseek-buffered-view-search-eof-string":
		return "tool_search", `"abc`, "EOF while parsing a string at line 1 column 4"
	case "deepseek-buffered-view-eof-string-surrogate":
		return "lookup", `"\uD800x`, "unexpected end of hex escape at line 1 column 8"
	case "deepseek-buffered-view-eof-string-surrogate-pair":
		return "lookup", `"\uD800\u0041`, "lone leading surrogate in hex escape at line 1 column 13"
	case "deepseek-buffered-view-eof-literal-value":
		return "lookup", `{"a":tru`, "EOF while parsing a value at line 1 column 8"
	case "deepseek-buffered-view-eof-string-early-range":
		return "lookup", `{"v":1e309,"s":"abc`, "number out of range at line 1 column 10"
	case "deepseek-buffered-view-search-output-surrogate-quote":
		return "tool_search", `"\uD800\""`, "unexpected end of hex escape at line 1 column 99"
	}
	return "", "", ""
}

func bufferedFragmentResponse04384(phase string) string {
	if bufferedSearchRuntimeDepthCase04384(phase) {
		return ""
	}
	name, arguments, diagnostic := bufferedFragmentFixture04384(phase)
	if diagnostic != "" {
		if strings.HasPrefix(phase, "deepseek-buffered-view-search-output-") {
			return bufferedToolFailedMessage04384("DeepSeek Mojo returned invalid response tool-call JSON: " + diagnostic)
		}
		return bufferedToolFailedResponse04384(name, diagnostic)
	}
	if name == "" {
		return ""
	}
	if name == "tool_search" {
		return bufferedToolSuccessResponse04384(map[string]any{"type": "tool_search_call", "call_id": "call-differential",
			"execution": "client", "arguments": json.RawMessage(arguments)})
	}
	item := map[string]any{"type": "function_call", "call_id": "call-differential", "name": name, "arguments": arguments}
	if name == "apply_patch" {
		item["type"], item["input"] = "custom_tool_call", arguments
		delete(item, "arguments")
	}
	return bufferedToolSuccessResponse04384(item)
}

func bufferedSearchRuntimeDepthCase04384(phase string) bool {
	return phase == "deepseek-buffered-view-search-runtime-depth-over" || phase == "deepseek-buffered-view-search-runtime-depth-over-rotation"
}
