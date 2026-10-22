package main

func deepSeekMessageArgumentSuffixes04384() []string {
	return []string{"message-arguments-absent", "message-arguments-null", "message-arguments-string", "message-arguments-string-escape", "message-arguments-object", "message-arguments-array", "message-arguments-u64", "message-arguments-input-ignored", "message-arguments-priority-null"}
}

func deepSeekMessageArgumentFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fields, argument := "", `"{}"`
	switch suffix {
	case "message-arguments-absent":
	case "message-arguments-null":
		fields, argument = `,"arguments":null`, `"null"`
	case "message-arguments-string":
		argument = `" { \"x\": 1 } "`
		fields = `,"arguments":` + argument
	case "message-arguments-string-escape":
		argument = `"{\"escaped\":\"\\\\u003c\"}"`
		fields = `,"arguments":` + argument
	case "message-arguments-object":
		text := "<>&/🌍\u2028\u2029"
		fields = `,"arguments":{"z":1e0,"a":"` + text + `"}`
		argument = `"{\"a\":\"` + text + `\",\"z\":1.0}"`
	case "message-arguments-array":
		fields, argument = `,"arguments":[null,false,1e0,-0]`, `"[null,false,1.0,-0.0]"`
	case "message-arguments-u64":
		fields, argument = `,"arguments":18446744073709551615`, `"18446744073709551615"`
	case "message-arguments-input-ignored":
		fields = `,"input":{"ignored":true}`
	case "message-arguments-priority-null":
		fields, argument = `,"arguments":null,"input":{"ignored":true}`, `"null"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[{"type":"message","role":"assistant","content":"preface","tool_calls":[{"id":"call_synthetic","type":"function","function":{"name":"lookup"` + fields + `}}]},{"type":"function_call_output","call_id":"call_synthetic","output":"done"},{"type":"message","role":"user","content":"same request"}]}`
	wire := `{"messages":[{"content":"preface","role":"assistant","tool_calls":[{"function":{"arguments":` + argument + `,"name":"lookup"},"id":"call_synthetic","type":"function"}]},{"content":"done","role":"tool","tool_call_id":"call_synthetic"},{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	return requestPrimitiveFixture04384{body: body, wireBody: wire}, true
}
