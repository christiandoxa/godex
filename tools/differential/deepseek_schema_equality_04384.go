package main

func deepSeekSchemaEqualitySuffixes04384() []string {
	return []string{"schema-equal-exact", "schema-equal-exponent", "schema-equal-signed-zero", "schema-equal-fraction", "schema-equal-int-float", "schema-equal-zero-kinds", "schema-equal-string-zero"}
}

func deepSeekSchemaEqualityFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	first, second, canonical, reject := "9007199254740993", "9007199254740993", "9007199254740993", false
	switch suffix {
	case "schema-equal-exact":
	case "schema-equal-exponent":
		first, second, canonical = "1e0", "1.0", "1.0"
	case "schema-equal-signed-zero":
		first, second, canonical = "-0.0", "0.0", "-0.0"
	case "schema-equal-fraction":
		first, second, canonical = "0.123456789012345678901234567890", "0.12345678901234568", "0.12345678901234568"
	case "schema-equal-int-float":
		first, second, reject = "1", "1.0", true
	case "schema-equal-zero-kinds":
		first, second, reject = "0", "-0", true
	case "schema-equal-string-zero":
		first, second, reject = `"-0.0"`, `"0.0"`, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	tool := func(value string) string {
		return `{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"value":{"enum":[` + value + `]}}}}`
	}
	return requestPrimitiveFixture04384{
		body:     `{"model":"gemini-3.1-pro-preview","input":"same request","stream":false,"tools":[` + tool(first) + `,` + tool(second) + `]}`,
		wireBody: `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false,"tools":[{"function":{"name":"lookup","parameters":{"properties":{"value":{"enum":[` + canonical + `]}},"type":"object"}},"type":"function"}]}`,
		reject:   reject,
	}, true
}
