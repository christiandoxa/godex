package main

func geminiMessageRoleSuffixes04384() []string {
	return []string{"message-role-user", "message-role-absent", "message-role-escaped", "message-role-null", "message-role-boolean", "message-role-number", "message-role-array", "message-role-object",
		"message-role-empty", "message-role-tool", "message-role-unknown", "message-role-padded", "message-role-tools", "message-role-candidate", "message-role-serde"}
}

func geminiMessageRoleFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{reject: true}
	role, extra := "null", ""
	switch suffix {
	case "message-role-user":
		role, fixture.reject = `"user"`, false
	case "message-role-absent":
		return requestPrimitiveFixture04384{body: `{"model":"gemini-3.1-pro-preview","input":[{"type":"message","content":"same request"}]}`}, true
	case "message-role-escaped":
		role, fixture.reject = `"\u0075ser"`, false
	case "message-role-null":
	case "message-role-boolean":
		role = "false"
	case "message-role-number":
		role = "7"
	case "message-role-array":
		role = "[]"
	case "message-role-object":
		role = "{}"
	case "message-role-empty":
		role = `""`
	case "message-role-tool":
		role = `"tool"`
	case "message-role-unknown":
		role = `"unknown"`
	case "message-role-padded":
		role = `" user "`
	case "message-role-tools":
		extra, fixture.geminiValidationError = `,"tools":false`, "invalid_tool_declaration: Gemini request field `tools` must be an array"
	case "message-role-candidate":
		extra, fixture.geminiValidationError = `,"candidate_count":2`, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "message-role-serde":
		extra = `,"unused":"\ud800"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":[{"type":"message","role":` + role + `,"content":"same request"}]` + extra + `}`
	return fixture, true
}
