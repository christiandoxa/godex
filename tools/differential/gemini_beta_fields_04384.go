package main

func geminiBetaFieldSuffixes04384() []string {
	return []string{"beta-control", "beta-prefix-null", "beta-prefix-false", "beta-prefix-empty", "beta-suffix-null", "beta-suffix-empty", "beta-prompt-null", "beta-prompt-empty", "beta-tools", "beta-candidate", "beta-serde"}
}

func geminiBetaFieldFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{reject: true}
	switch suffix {
	case "beta-control":
		fixture.fields, fixture.reject = `"prefix_hint":false,"unused":{"prefix":null,"suffix":null,"prompt":null}`, false
	case "beta-prefix-null":
		fixture.fields = `"prefix":null`
	case "beta-prefix-false":
		fixture.fields = `"prefix":false`
	case "beta-prefix-empty":
		fixture.fields = `"prefix":""`
	case "beta-suffix-null":
		fixture.fields = `"suffix":null`
	case "beta-suffix-empty":
		fixture.fields = `"suffix":""`
	case "beta-prompt-null":
		fixture.fields = `"prompt":null`
	case "beta-prompt-empty":
		fixture.fields = `"prompt":""`
	case "beta-tools":
		fixture.fields, fixture.geminiValidationError = `"prefix":null,"tools":false`, "invalid_tool_declaration: Gemini request field `tools` must be an array"
	case "beta-candidate":
		fixture.fields, fixture.geminiValidationError = `"prefix":null,"candidate_count":2`, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "beta-serde":
		fixture.fields = `"prefix":null,"unused":"\ud800"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	return fixture, true
}
