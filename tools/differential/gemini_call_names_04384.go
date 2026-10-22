package main

func geminiCallNameSuffixes04384() []string {
	return []string{"call-name-control", "call-name-missing", "call-name-null", "call-name-number", "call-name-blank", "call-name-shadow-empty", "call-name-custom", "call-name-mcp", "call-name-tools", "call-name-candidate", "call-name-serde"}
}

func geminiCallNameFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{reject: true}
	kind, name, extra := "function_call", "", ""
	switch suffix {
	case "call-name-control":
		name, fixture.reject = `,"name":"synthetic_lookup"`, false
	case "call-name-missing":
	case "call-name-null":
		name = `,"name":null`
	case "call-name-number":
		name = `,"name":7`
	case "call-name-blank":
		name = `,"name":" \t"`
	case "call-name-shadow-empty":
		name = `,"name":"","tool_name":"later"`
	case "call-name-custom":
		kind = "custom_tool_call"
	case "call-name-mcp":
		kind = "mcp_call"
	case "call-name-tools":
		extra, fixture.geminiValidationError = `,"tools":false`, "invalid_tool_declaration: Gemini request field `tools` must be an array"
	case "call-name-candidate":
		extra, fixture.geminiValidationError = `,"candidate_count":2`, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "call-name-serde":
		extra = `,"unused":"\ud800"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"type":"` + kind + `","call_id":"synthetic-call","arguments":"{}","input":"synthetic"` + name + `}]` + extra + `}`
	return fixture, true
}
