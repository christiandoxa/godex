package main

func geminiCustomCallSuffixes04384() []string {
	return []string{"custom-call-control", "custom-call-missing-input", "custom-call-null-input", "custom-call-false-input", "custom-call-object-input", "custom-call-array-input", "custom-call-unicode-input",
		"custom-call-tool-id", "custom-call-id", "custom-call-null-id", "custom-call-tool-name", "custom-call-null-name", "custom-call-nested-name", "custom-call-sentinel-name", "custom-call-ignored-arguments", "custom-call-namespace"}
}

func geminiCustomCallFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	identity, wireID := `"call_id":"synthetic-call"`, "synthetic-call"
	name, input, extra := `"name":"synthetic_lookup"`, `,"input":"synthetic-input"`, ""
	arguments, emptyCall := `{\"input\":\"synthetic-input\"}`, false
	switch suffix {
	case "custom-call-control":
	case "custom-call-missing-input", "custom-call-ignored-arguments":
		input, arguments = "", `{\"input\":\"\"}`
		if suffix == "custom-call-ignored-arguments" {
			extra = `,"arguments":"ignored"`
		}
	case "custom-call-null-input":
		input, arguments = `,"input":null`, `{\"input\":\"null\"}`
	case "custom-call-false-input":
		input, arguments = `,"input":false`, `{\"input\":\"false\"}`
	case "custom-call-object-input":
		input, arguments = `,"input":{"b":1.00,"a":"<"}`, `{\"input\":\"{\\\"a\\\":\\\"<\\\",\\\"b\\\":1.0}\"}`
	case "custom-call-array-input":
		input, arguments = `,"input":["<",1.00,null]`, `{\"input\":\"[\\\"<\\\",1.0,null]\"}`
	case "custom-call-unicode-input":
		input, arguments = `,"input":" <&🌍 "`, `{\"input\":\" <&🌍 \"}`
	case "custom-call-tool-id":
		identity = `"tool_call_id":"synthetic-call"`
	case "custom-call-id":
		identity = `"id":"synthetic-call"`
	case "custom-call-null-id":
		identity, wireID = `"call_id":null,"tool_call_id":"synthetic-call"`, "call_1"
	case "custom-call-tool-name":
		name = `"tool_name":"synthetic_lookup"`
	case "custom-call-null-name":
		name, emptyCall = `"name":null,"tool_name":"synthetic_lookup"`, true
	case "custom-call-nested-name":
		name, emptyCall = `"function":{"name":"synthetic_lookup"}`, true
	case "custom-call-sentinel-name":
		name, emptyCall = `"__missing__":"synthetic_lookup"`, true
	case "custom-call-namespace":
		extra = `,"namespace":"ignored"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"type":"custom_tool_call",` + identity + `,` + name + input + extra + `},{"type":"function_call_output",` + identity + `,"output":"synthetic-result"}]}`
	messages := `{"content":"same request","role":"user"},{"content":"","role":"assistant"}`
	if !emptyCall {
		messages = `{"content":"same request","role":"user"},{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"` + arguments + `","name":"synthetic_lookup"},"id":"` + wireID + `","type":"function"}]},{"content":"synthetic-result","role":"tool","tool_call_id":"` + wireID + `"}`
	}
	return requestPrimitiveFixture04384{body: body, wireBody: `{"messages":[` + messages + `],"model":"gemini-3.1-pro-preview","stream":false}`}, true
}
