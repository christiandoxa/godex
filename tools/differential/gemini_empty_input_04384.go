package main

func geminiEmptyInputSuffixes04384() []string {
	return []string{"empty-input-absent", "empty-input-array", "empty-input-whitespace", "empty-input-blank-instructions", "empty-input-string", "empty-input-system",
		"empty-input-null", "empty-input-object", "empty-input-candidate", "empty-input-serde"}
}

func geminiEmptyInputFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fields := ""
	fixture := requestPrimitiveFixture04384{wireBody: `{"messages":[{"content":"","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`}
	switch suffix {
	case "empty-input-absent":
	case "empty-input-array":
		fields = `,"input":[]`
	case "empty-input-whitespace":
		fields = `,"instructions":" \t\u2003"`
	case "empty-input-blank-instructions":
		fields = `,"instructions":"","input":[]`
	case "empty-input-string":
		fields = `,"input":""`
	case "empty-input-system":
		fields = `,"instructions":"same request","input":[]`
		fixture.wireBody = `{"messages":[{"content":"same request","role":"system"}],"model":"gemini-3.1-pro-preview","stream":false}`
	case "empty-input-null":
		fields, fixture.reject = `,"input":null`, true
	case "empty-input-object":
		fields, fixture.reject = `,"input":{}`, true
	case "empty-input-candidate":
		fields, fixture.reject, fixture.geminiValidationError = `,"candidate_count":2`, true, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "empty-input-serde":
		fields, fixture.reject = `,"unused":"\ud800"`, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview"` + fields + `}`
	return fixture, true
}
