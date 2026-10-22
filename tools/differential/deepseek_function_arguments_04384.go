package main

import "strings"

func deepSeekFunctionArgumentSuffixes04384() []string {
	return []string{"function-arguments-absent", "function-arguments-null", "function-arguments-string", "function-arguments-string-escape", "function-arguments-input", "function-arguments-nested", "function-arguments-nested-null", "function-arguments-priority-null", "function-arguments-object", "function-arguments-array", "function-arguments-u64", "function-arguments-normal-absent", "function-arguments-normal-object", "function-arguments-float", "function-arguments-negative-zero", "function-arguments-mcp-absent", "function-arguments-mcp-priority-null", "function-arguments-mcp-nested", "function-arguments-custom-control"}
}

func deepSeekFunctionArgumentFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fields, argument := `"name":"lookup"`, `"{}"`
	switch suffix {
	case "function-arguments-absent", "function-arguments-normal-absent", "function-arguments-mcp-absent":
	case "function-arguments-null":
		fields += `,"arguments":null`
		argument = `"null"`
	case "function-arguments-string":
		argument = `" { \"x\": 1 } "`
		fields += `,"arguments":` + argument
	case "function-arguments-string-escape":
		argument = `"{\"escaped\":\"\\\\u003c\"}"`
		fields += `,"arguments":` + argument
	case "function-arguments-input":
		fields += `,"input":{"v":1}`
		argument = `"{\"v\":1}"`
	case "function-arguments-nested", "function-arguments-mcp-nested":
		fields = `"function":{"name":"lookup","arguments":{"v":1}}`
		argument = `"{\"v\":1}"`
	case "function-arguments-nested-null":
		fields = `"function":{"name":"lookup","arguments":null}`
		argument = `"null"`
	case "function-arguments-priority-null", "function-arguments-mcp-priority-null":
		fields += `,"arguments":null,"input":{"v":1},"function":{"name":"lookup","arguments":{"v":2}}`
		argument = `"null"`
	case "function-arguments-object", "function-arguments-normal-object":
		text := "<>&/🌍\u2028\u2029"
		fields += `,"arguments":{"text":"` + text + `"}`
		argument = `"{\"text\":\"` + text + `\"}"`
	case "function-arguments-array":
		fields += `,"arguments":[null,false,1]`
		argument = `"[null,false,1]"`
	case "function-arguments-u64":
		fields += `,"arguments":{"n":18446744073709551615}`
		argument = `"{\"n\":18446744073709551615}"`
	case "function-arguments-float":
		fields += `,"arguments":{"n":1e0}`
		argument = `"{\"n\":1.0}"`
	case "function-arguments-negative-zero":
		fields += `,"arguments":-0`
		argument = `"-0.0"`
	case "function-arguments-custom-control":
		fields += `,"input":"synthetic patch"`
		argument = `"{\"input\":\"synthetic patch\"}"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	inputPrefix, messagesPrefix := "", ""
	if strings.Contains(suffix, "-normal-") {
		inputPrefix = `{"type":"message","role":"developer","content":"synthetic instruction"},`
		messagesPrefix = `{"content":"synthetic instruction","role":"system"},`
	}
	kind := "function_call"
	if strings.Contains(suffix, "-mcp-") {
		kind = "mcp_call"
	} else if suffix == "function-arguments-custom-control" {
		kind = "custom_tool_call"
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[` + inputPrefix + `{"type":"` + kind + `","call_id":"call_synthetic",` + fields + `},{"type":"function_call_output","call_id":"call_synthetic","output":"done"},{"type":"message","role":"user","content":"same request"}]}`
	wire := `{"messages":[` + messagesPrefix + `{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":` + argument + `,"name":"lookup"},"id":"call_synthetic","type":"function"}]},{"content":"done","role":"tool","tool_call_id":"call_synthetic"},{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	return requestPrimitiveFixture04384{body: body, wireBody: wire}, true
}
