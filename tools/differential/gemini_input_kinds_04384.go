package main

func geminiInputKindSuffixes04384() []string {
	return []string{"input-kind-message", "input-kind-implicit", "input-kind-null", "input-kind-boolean", "input-kind-number", "input-kind-array", "input-kind-object", "input-kind-empty",
		"input-kind-unknown", "input-kind-text", "input-kind-output-text", "input-kind-image", "input-kind-reasoning", "input-kind-tools", "input-kind-candidate", "input-kind-serde"}
}

func geminiInputKindFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{reject: true}
	kind, extra := `"unknown"`, ""
	switch suffix {
	case "input-kind-message":
		kind, fixture.reject = `"message"`, false
	case "input-kind-implicit":
		return requestPrimitiveFixture04384{body: `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"}]}`}, true
	case "input-kind-null":
		kind = "null"
	case "input-kind-boolean":
		kind = "false"
	case "input-kind-number":
		kind = "7"
	case "input-kind-array":
		kind = "[]"
	case "input-kind-object":
		kind = "{}"
	case "input-kind-empty":
		kind = `""`
	case "input-kind-unknown":
	case "input-kind-text":
		kind = `"input_text"`
	case "input-kind-output-text":
		kind = `"output_text"`
	case "input-kind-image":
		kind = `"input_image"`
	case "input-kind-reasoning":
		kind = `"reasoning"`
	case "input-kind-tools":
		extra, fixture.geminiValidationError = `,"tools":false`, "invalid_tool_declaration: Gemini request field `tools` must be an array"
	case "input-kind-candidate":
		extra, fixture.geminiValidationError = `,"candidate_count":2`, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "input-kind-serde":
		extra = `,"unused":"\ud800"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":[{"type":` + kind + `,"role":"user","content":"same request","text":"same request"}]` + extra + `}`
	return fixture, true
}
