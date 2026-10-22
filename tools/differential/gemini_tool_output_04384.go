package main

func geminiToolOutputSuffixes04384() []string {
	return []string{"tool-output-missing", "tool-output-custom-missing", "tool-output-mcp-missing", "tool-output-mcp-call-missing",
		"tool-output-null", "tool-output-empty", "tool-output-string", "tool-output-tools", "tool-output-candidate", "tool-output-serde"}
}

func geminiToolOutputFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{reject: true}
	kind, output, text, extra := "function_call_output", "", "", ""
	switch suffix {
	case "tool-output-missing":
	case "tool-output-custom-missing":
		kind = "custom_tool_call_output"
	case "tool-output-mcp-missing":
		kind = "mcp_tool_result"
	case "tool-output-mcp-call-missing":
		kind = "mcp_call_output"
	case "tool-output-null":
		output, text, fixture.reject = `,"output":null`, "null", false
	case "tool-output-empty":
		output, fixture.reject = `,"output":""`, false
	case "tool-output-string":
		output, text, fixture.reject = `,"output":"synthetic-result"`, "synthetic-result", false
	case "tool-output-tools":
		extra, fixture.geminiValidationError = `,"tools":false`, "invalid_tool_declaration: Gemini request field `tools` must be an array"
	case "tool-output-candidate":
		extra, fixture.geminiValidationError = `,"candidate_count":2`, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "tool-output-serde":
		extra = `,"unused":"\ud800"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	inputCall, wireCall := "", ""
	if !fixture.reject {
		inputCall = `,{"type":"function_call","call_id":"synthetic-call","name":"synthetic_lookup","arguments":"{}"}`
		wireCall = `,{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"{}","name":"synthetic_lookup"},"id":"synthetic-call","type":"function"}]}`
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"}` + inputCall + `,{"type":"` + kind + `","call_id":"synthetic-call"` + output + `}]` + extra + `}`
	fixture.wireBody = `{"messages":[{"content":"same request","role":"user"}` + wireCall + `,{"content":"` + text + `","role":"tool","tool_call_id":"synthetic-call"}],"model":"gemini-3.1-pro-preview","stream":false}`
	return fixture, true
}
