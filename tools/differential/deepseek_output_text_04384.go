package main

import "strings"

func deepSeekOutputTextSuffixes04384() []string {
	return []string{"output-text-string", "output-text-null", "output-text-object", "output-text-empty-array", "output-text-strings", "output-text-parts", "output-text-empty-parts", "output-text-aliases", "output-text-skipped", "output-text-normal-parts", "output-text-custom-parts", "output-text-mcp-parts", "output-text-mcp-inline", "output-text-mcp-inline-null", "output-text-mcp-inline-object", "output-text-mcp-inline-string"}
}

func deepSeekOutputTextFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	value, content := `[{"type":"output_text","text":"first"},{"input_text":"second"},{"output_text":"third"}]`, `"first\nsecond\nthird"`
	switch suffix {
	case "output-text-string":
		value, content = `"synthetic result"`, `"synthetic result"`
	case "output-text-null":
		value, content = `null`, `"null"`
	case "output-text-object":
		value, content = `{"z":1e0,"a":"<>&/🌍"}`, `"{\"a\":\"<>&/🌍\",\"z\":1.0}"`
	case "output-text-empty-array":
		value, content = `[]`, `""`
	case "output-text-strings":
		value, content = `["first","second"]`, `"first\nsecond"`
	case "output-text-parts", "output-text-normal-parts", "output-text-custom-parts", "output-text-mcp-parts":
	case "output-text-mcp-inline":
		content = `[{"text":"first","type":"output_text"},{"input_text":"second"},{"output_text":"third"}]`
	case "output-text-mcp-inline-null":
		value, content = `null`, `null`
	case "output-text-mcp-inline-object":
		value, content = `{"z":1e0,"a":"<>&/🌍"}`, `{"a":"<>&/🌍","z":1.0}`
	case "output-text-mcp-inline-string":
		value, content = `"synthetic result"`, `"synthetic result"`
	case "output-text-empty-parts":
		value, content = `["",{"text":""},{"text":"last"}]`, `"\n\nlast"`
	case "output-text-aliases":
		value = `[{"text":null,"input_text":"first","output_text":"ignored"},{"text":"","input_text":"ignored"},{"text":1,"input_text":false,"output_text":"third"}]`
		content = `"first\n\nthird"`
	case "output-text-skipped":
		value, content = `[null,false,7,[],{},"last"]`, `"last"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	inputPrefix, messagesPrefix := "", ""
	if strings.Contains(suffix, "-normal-") {
		inputPrefix = `{"type":"message","role":"developer","content":"synthetic instruction"},`
		messagesPrefix = `{"content":"synthetic instruction","role":"system"},`
	}
	kind, outputKind := "function_call", "function_call_output"
	if strings.Contains(suffix, "-custom-") {
		outputKind = "custom_tool_call_output"
	} else if strings.Contains(suffix, "-mcp-") {
		outputKind = "mcp_call_output"
	}
	output := `},{"type":"` + outputKind + `","call_id":"call_synthetic","output":` + value + `}`
	if strings.HasPrefix(suffix, "output-text-mcp-inline") {
		kind, output = "mcp_call", `,"output":`+value+`}`
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[` + inputPrefix + `{"type":"` + kind + `","call_id":"call_synthetic","name":"lookup","arguments":"{}"` + output + `,{"type":"message","role":"user","content":"same request"}]}`
	wire := `{"messages":[` + messagesPrefix + `{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"{}","name":"lookup"},"id":"call_synthetic","type":"function"}]},{"content":` + content + `,"role":"tool","tool_call_id":"call_synthetic"},{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	return requestPrimitiveFixture04384{body: body, wireBody: wire}, true
}
