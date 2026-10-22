package main

import "strings"

func deepSeekCustomInputSuffixes04384() []string {
	return []string{"custom-input-absent", "custom-input-null", "custom-input-string", "custom-input-empty", "custom-input-escaped", "custom-input-unicode", "custom-input-object", "custom-input-array", "custom-input-bool", "custom-input-u64", "custom-input-arguments-only", "custom-input-priority", "custom-input-null-priority", "custom-input-nested-only", "custom-input-normal-absent", "custom-input-normal-null", "custom-input-normal-object", "custom-input-normal-priority"}
}

func deepSeekCustomInputFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fields, argument := "", `"{\"input\":\"\"}"`
	switch suffix {
	case "custom-input-absent", "custom-input-normal-absent":
	case "custom-input-null", "custom-input-normal-null":
		fields, argument = `,"input":null`, `"{\"input\":\"null\"}"`
	case "custom-input-string":
		fields, argument = `,"input":"synthetic patch"`, `"{\"input\":\"synthetic patch\"}"`
	case "custom-input-empty":
		fields = `,"input":""`
	case "custom-input-escaped":
		fields = `,"input":"line\n\"quoted\"\\slash"`
		argument = `"{\"input\":\"line\\n\\\"quoted\\\"\\\\slash\"}"`
	case "custom-input-unicode":
		text := "<>&/🌍\u2028\u2029"
		fields, argument = `,"input":"`+text+`"`, `"{\"input\":\"`+text+`\"}"`
	case "custom-input-object", "custom-input-normal-object":
		text := "<>&/🌍\u2028\u2029"
		fields = `,"input":{"z":1e0,"a":"` + text + `"}`
		argument = `"{\"input\":\"{\\\"a\\\":\\\"` + text + `\\\",\\\"z\\\":1.0}\"}"`
	case "custom-input-array":
		fields, argument = `,"input":[null,false,1e0,-0]`, `"{\"input\":\"[null,false,1.0,-0.0]\"}"`
	case "custom-input-bool":
		fields, argument = `,"input":false`, `"{\"input\":\"false\"}"`
	case "custom-input-u64":
		fields, argument = `,"input":18446744073709551615`, `"{\"input\":\"18446744073709551615\"}"`
	case "custom-input-arguments-only":
		fields = `,"arguments":"ignored"`
	case "custom-input-priority", "custom-input-normal-priority":
		fields, argument = `,"arguments":"ignored","input":"synthetic patch"`, `"{\"input\":\"synthetic patch\"}"`
	case "custom-input-null-priority":
		fields, argument = `,"arguments":"ignored","input":null`, `"{\"input\":\"null\"}"`
	case "custom-input-nested-only":
		fields = `,"function":{"arguments":{"ignored":true}}`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	inputPrefix, messagesPrefix := "", ""
	if strings.Contains(suffix, "-normal-") {
		inputPrefix = `{"type":"message","role":"developer","content":"synthetic instruction"},`
		messagesPrefix = `{"content":"synthetic instruction","role":"system"},`
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[` + inputPrefix + `{"type":"custom_tool_call","call_id":"call_synthetic","name":"lookup"` + fields + `},{"type":"custom_tool_call_output","call_id":"call_synthetic","output":"done"},{"type":"message","role":"user","content":"same request"}]}`
	wire := `{"messages":[` + messagesPrefix + `{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":` + argument + `,"name":"lookup"},"id":"call_synthetic","type":"function"}]},{"content":"done","role":"tool","tool_call_id":"call_synthetic"},{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	return requestPrimitiveFixture04384{body: body, wireBody: wire}, true
}
