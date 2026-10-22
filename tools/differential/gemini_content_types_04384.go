package main

func geminiContentTypeSuffixes04384() []string {
	return []string{"message-content-string", "message-content-array", "message-content-null", "message-content-boolean", "message-content-number", "message-content-tools", "message-content-candidate", "message-content-serde"}
}

func geminiContentTypeFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{reject: true}
	content, extra := "null", ""
	switch suffix {
	case "message-content-string":
		content, fixture.reject = `"same request"`, false
	case "message-content-array":
		content, fixture.reject = `[{"type":"input_text","text":"same request"}]`, false
	case "message-content-null":
	case "message-content-boolean":
		content = "false"
	case "message-content-number":
		content = "7"
	case "message-content-tools":
		extra, fixture.geminiValidationError = `,"tools":false`, "invalid_tool_declaration: Gemini request field `tools` must be an array"
	case "message-content-candidate":
		extra, fixture.geminiValidationError = `,"candidate_count":2`, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "message-content-serde":
		extra = `,"unused":"\ud800"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	items := `{"type":"message","role":"user","content":` + content + `}`
	if fixture.reject {
		items += `,{"role":"user","content":"same request"}`
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":[` + items + `]` + extra + `}`
	return fixture, true
}
