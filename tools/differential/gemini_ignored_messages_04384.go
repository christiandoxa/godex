package main

func geminiIgnoredMessagesSuffixes04384() []string {
	return []string{"ignored-messages-null", "ignored-messages-false", "ignored-messages-object", "ignored-messages-array", "ignored-messages-string", "ignored-messages-absent-input", "ignored-messages-null-input", "ignored-messages-candidate", "ignored-messages-serde"}
}

func geminiIgnoredMessagesFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	value, input := "null", `,"input":"same request"`
	fixture := requestPrimitiveFixture04384{wireBody: `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`}
	switch suffix {
	case "ignored-messages-null":
	case "ignored-messages-false":
		value = "false"
	case "ignored-messages-object":
		value = `{"role":"user","content":"ignored"}`
	case "ignored-messages-array":
		value = `[{"role":"system","content":"ignored"}]`
	case "ignored-messages-string":
		value = `"ignored"`
	case "ignored-messages-absent-input":
		value, input = `[{"role":"user","content":"ignored"}]`, ""
		fixture.wireBody = `{"messages":[{"content":"","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	case "ignored-messages-null-input":
		input, fixture.reject = `,"input":null`, true
	case "ignored-messages-candidate":
		input, fixture.reject, fixture.geminiValidationError = `,"candidate_count":2`, true, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "ignored-messages-serde":
		value, fixture.reject = `"\ud800"`, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","messages":` + value + input + `}`
	return fixture, true
}
