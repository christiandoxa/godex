package main

func geminiFunctionAliasSuffixes04384() []string {
	return []string{"function-alias-control", "function-alias-tool-id", "function-alias-id", "function-alias-null-id", "function-alias-tool-name", "function-alias-nested-name",
		"function-alias-missing-arguments", "function-alias-null-arguments", "function-alias-input-arguments", "function-alias-nested-arguments", "function-alias-canonical-arguments", "function-alias-namespace",
		"function-alias-content-output", "function-alias-result-output", "function-alias-output-text", "function-alias-null-name", "function-alias-number-name", "function-alias-sentinel-name"}
}

func geminiFunctionAliasFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	identity, wireID := `"call_id":"synthetic-call"`, "synthetic-call"
	name, arguments, wireArguments := `"name":"synthetic_lookup"`, `"arguments":"{}"`, `{}`
	output, wireOutput, extra := `"output":"synthetic-result"`, "synthetic-result", ""
	emptyCall := false
	switch suffix {
	case "function-alias-control":
	case "function-alias-tool-id":
		identity = `"tool_call_id":"synthetic-call"`
	case "function-alias-id":
		identity = `"id":"synthetic-call"`
	case "function-alias-null-id":
		identity, wireID = `"call_id":null,"tool_call_id":"synthetic-call"`, "call_1"
	case "function-alias-tool-name":
		name = `"tool_name":"synthetic_lookup"`
	case "function-alias-nested-name":
		name = `"function":{"name":"synthetic_lookup"}`
	case "function-alias-missing-arguments":
		arguments = ""
	case "function-alias-null-arguments":
		arguments, wireArguments = `"arguments":null,"input":"ignored"`, "null"
	case "function-alias-input-arguments":
		arguments, wireArguments = `"input":"synthetic-input"`, "synthetic-input"
	case "function-alias-nested-arguments":
		arguments, wireArguments = `"function":{"arguments":"synthetic-input"}`, "synthetic-input"
	case "function-alias-canonical-arguments":
		arguments, wireArguments = `"arguments":{"b":1.00,"a":"<"}`, `{\"a\":\"<\",\"b\":1.0}`
	case "function-alias-namespace":
		extra = `,"namespace":"ignored"`
	case "function-alias-content-output":
		output = `"content":"synthetic-result"`
	case "function-alias-result-output":
		output = `"result":"synthetic-result"`
	case "function-alias-output-text":
		output, wireOutput = `"output":["first",{"text":""},{"input_text":"last"}]`, `first\n\nlast`
	case "function-alias-null-name":
		name, emptyCall = `"name":null,"tool_name":"synthetic_lookup"`, true
	case "function-alias-number-name":
		name, emptyCall = `"name":7,"function":{"name":"synthetic_lookup"}`, true
	case "function-alias-sentinel-name":
		name, emptyCall = `"__missing__":"synthetic_lookup"`, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	if arguments != "" {
		arguments = "," + arguments
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"type":"function_call",` + identity + `,` + name + arguments + extra + `},{"type":"function_call_output",` + identity + `,` + output + `}]}`
	wire := `{"messages":[{"content":"same request","role":"user"},{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"` + wireArguments + `","name":"synthetic_lookup"},"id":"` + wireID + `","type":"function"}]},{"content":"` + wireOutput + `","role":"tool","tool_call_id":"` + wireID + `"}],"model":"gemini-3.1-pro-preview","stream":false}`
	if emptyCall {
		wire = `{"messages":[{"content":"same request","role":"user"},{"content":"","role":"assistant"}],"model":"gemini-3.1-pro-preview","stream":false}`
	}
	return requestPrimitiveFixture04384{body: body, wireBody: wire}, true
}
