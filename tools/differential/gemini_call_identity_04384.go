package main

func geminiCallIdentitySuffixes04384() []string {
	return []string{"call-id-control", "call-id-missing", "call-id-null", "call-id-number", "call-id-blank", "call-id-shadow-empty", "call-id-custom", "call-id-mcp", "call-id-shell",
		"call-id-output", "call-id-custom-output", "call-id-mcp-result", "call-id-mcp-output", "call-id-tools", "call-id-candidate", "call-id-serde"}
}

func geminiCallIdentityFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{reject: true}
	kind, identity, extra := "function_call", "", ""
	switch suffix {
	case "call-id-control":
		identity, fixture.reject = `,"call_id":"synthetic-call"`, false
	case "call-id-missing":
	case "call-id-null":
		identity = `,"call_id":null`
	case "call-id-number":
		identity = `,"call_id":7`
	case "call-id-blank":
		identity = `,"call_id":" \t"`
	case "call-id-shadow-empty":
		identity = `,"call_id":"","id":"later"`
	case "call-id-custom":
		kind = "custom_tool_call"
	case "call-id-mcp":
		kind = "mcp_call"
	case "call-id-shell":
		kind = "local_shell_call"
	case "call-id-output":
		kind = "function_call_output"
	case "call-id-custom-output":
		kind = "custom_tool_call_output"
	case "call-id-mcp-result":
		kind = "mcp_tool_result"
	case "call-id-mcp-output":
		kind = "mcp_call_output"
	case "call-id-tools":
		extra, fixture.geminiValidationError = `,"tools":false`, "invalid_tool_declaration: Gemini request field `tools` must be an array"
	case "call-id-candidate":
		extra, fixture.geminiValidationError = `,"candidate_count":2`, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "call-id-serde":
		extra = `,"unused":"\ud800"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"type":"` + kind + `","name":"synthetic_lookup","arguments":"{}","input":"synthetic","action":{"command":["synthetic"]},"output":"synthetic"` + identity + `}]` + extra + `}`
	return fixture, true
}
