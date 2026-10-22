package main

func geminiInputItemSuffixes04384() []string {
	return []string{"input-item-control", "input-item-metadata", "input-item-null", "input-item-boolean", "input-item-number", "input-item-string", "input-item-array", "input-item-trailing-number",
		"input-item-prefix-null", "input-item-prefix-false", "input-item-prefix-function", "input-item-prefix-text", "input-item-tools", "input-item-candidate", "input-item-serde"}
}

func geminiInputItemFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	const message = `{"role":"user","content":"same request"}`
	items, extra := message, ""
	fixture := requestPrimitiveFixture04384{reject: true}
	switch suffix {
	case "input-item-control":
		fixture.reject = false
	case "input-item-metadata":
		items, fixture.reject = `{"role":"user","content":"same request","suffix":null,"prompt":null,"metadata":{"prefix":null}}`, false
	case "input-item-null":
		items = `null,` + message
	case "input-item-boolean":
		items = `false,` + message
	case "input-item-number":
		items = `7,` + message
	case "input-item-string":
		items = `"discarded",` + message
	case "input-item-array":
		items = `[],` + message
	case "input-item-trailing-number":
		items = message + `,7`
	case "input-item-prefix-null":
		items = `{"role":"user","content":"same request","prefix":null}`
	case "input-item-prefix-false":
		items = `{"type":"message","role":"user","content":"same request","prefix":false}`
	case "input-item-prefix-function":
		items = message + `,{"type":"function_call","call_id":"synthetic-call","name":"synthetic_lookup","arguments":"{}","prefix":false}`
	case "input-item-prefix-text":
		items = `{"type":"input_text","text":"same request","prefix":null}`
	case "input-item-tools":
		items, extra = `7,`+message, `,"tools":false`
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools` must be an array"
	case "input-item-candidate":
		items, extra = `7,`+message, `,"candidate_count":2`
		fixture.geminiValidationError = "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	case "input-item-serde":
		items, extra = `7,`+message, `,"unused":"\ud800"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":[` + items + `]` + extra + `}`
	return fixture, true
}
