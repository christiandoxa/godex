package main

func deepSeekCloneNumberSuffixes04384() []string {
	return []string{"clone-number-tool-small", "clone-number-tool-i53", "clone-number-tool-u64", "clone-number-tool-i64", "clone-number-tool-float-control", "clone-number-tool-normal", "clone-number-tool-nested", "clone-number-tool-distinct", "clone-number-metadata-u64", "clone-number-metadata-float-control", "clone-number-client-i53", "clone-number-metadata-nested"}
}

func deepSeekCloneNumberFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	raw, number := "9007199254740993", "9007199254740993"
	fields, metadata, wireExtra := "", "", ""
	switch suffix {
	case "clone-number-tool-small":
		raw, number = "7", "7"
	case "clone-number-tool-i53", "clone-number-tool-normal", "clone-number-tool-nested", "clone-number-tool-distinct":
	case "clone-number-tool-u64":
		raw, number = "18446744073709551615", "18446744073709551615"
	case "clone-number-tool-i64":
		raw, number = "-9223372036854775808", "-9223372036854775808"
	case "clone-number-tool-float-control":
		raw, number = "1e0,-0,0.123456789012345678901234567890", "1.0,-0.0,0.12345678901234568"
	case "clone-number-metadata-u64":
		fields = `,"metadata":{"probe":{"value":18446744073709551615}}`
		metadata = `{"deepseek":{"finish_reason":"stop"},"probe":{"value":18446744073709551615}}`
	case "clone-number-metadata-float-control":
		fields = `,"metadata":{"probe":{"value":1.5}}`
		metadata = `{"deepseek":{"finish_reason":"stop"},"probe":{"value":1.5}}`
	case "clone-number-client-i53":
		fields = `,"client_metadata":{"value":9007199254740993}`
		metadata = `{"client_metadata":{"value":9007199254740993},"deepseek":{"finish_reason":"stop"}}`
	case "clone-number-metadata-nested":
		fields = `,"metadata":{"deepseek":{"probe":{"value":[18446744073709551615,-9223372036854775808]}},"probe":[{"value":9007199254740993}]}`
		metadata = `{"deepseek":{"finish_reason":"stop","probe":{"value":[18446744073709551615,-9223372036854775808]}},"probe":[{"value":9007199254740993}]}`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	if metadata == "" {
		parameters := `{"type":"object","properties":{"value":{"type":"integer","enum":[` + raw + `]}}}`
		tool := `{"type":"function","name":"lookup","parameters":` + parameters + `}`
		if suffix == "clone-number-tool-nested" {
			tool = `{"type":"function","function":{"name":"lookup","parameters":` + parameters + `}}`
		}
		if suffix == "clone-number-tool-distinct" {
			tool += `,{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"value":{"type":"integer","enum":[9007199254740992]}}}}`
		}
		fields = `,"tools":[` + tool + `]`
		wireExtra = `,"tools":[{"function":{"name":"lookup","parameters":{"properties":{"value":{"enum":[` + number + `],"type":"integer"}},"type":"object"}},"type":"function"}]`
	}
	if suffix == "clone-number-tool-normal" {
		fields += `,"safety_identifier":"synthetic_user"`
		wireExtra += `,"user_id":"synthetic_user"`
	}
	return requestPrimitiveFixture04384{
		body:             `{"model":"gemini-3.1-pro-preview","input":"same request","stream":false` + fields + `}`,
		wireBody:         `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false` + wireExtra + `}`,
		deepSeekMetadata: metadata,
		reject:           suffix == "clone-number-tool-distinct",
	}, true
}
