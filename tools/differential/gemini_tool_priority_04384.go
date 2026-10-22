package main

func geminiToolPriorityFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	input, extra := `"same request"`, ""
	switch suffix {
	case "tool-priority-stream":
		extra = `,"stream":null`
	case "tool-priority-temperature":
		extra = `,"temperature":null`
	case "tool-priority-tokens":
		extra = `,"max_tokens":0`
	case "tool-priority-user":
		extra = `,"user_id":null`
	case "tool-priority-input":
		input = "7"
	case "tool-priority-reasoning":
		extra = `,"reasoning":"opaque"`
	case "tool-priority-serde":
		extra = `,"unused":"\ud800"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture := requestPrimitiveFixture04384{reject: true,
		body:            `{"model":"gemini-3.1-pro-preview","input":` + input + `,"tools":[{"type":"function","function":{"name":"synthetic_lookup","parameters":false}}]` + extra + `}`,
		geminiToolError: "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"}
	if suffix == "tool-priority-serde" {
		fixture.geminiToolError = ""
	}
	return fixture, true
}
