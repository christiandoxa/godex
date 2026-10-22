package main

import "strings"

func geminiToolKindSuffixes04384() []string {
	return []string{"tool-kind-missing", "tool-kind-null", "tool-kind-number", "tool-kind-array", "tool-kind-empty", "tool-kind-unknown",
		"tool-kind-nested", "tool-kind-nested-null-type", "tool-kind-nested-number-type", "tool-kind-nested-empty-type", "tool-kind-nested-unknown-type",
		"tool-kind-function-null", "tool-kind-function-name", "tool-kind-mcp", "tool-kind-mcp-alias", "tool-kind-mcp-number-type",
		"tool-kind-mcp-metadata", "tool-kind-mcp-strict", "tool-kind-mcp-null-schema", "tool-kind-mcp-missing-schema", "tool-kind-mcp-invalid-name",
		"tool-kind-mcp-unknown-type", "tool-kind-mcp-nested", "tool-kind-mcp-empty-segment", "tool-kind-media", "tool-kind-media-unknown-type",
		"tool-kind-mixed", "tool-kind-choice", "tool-kind-choice-missing", "tool-kind-stream"}
}

func geminiToolKindFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	const nested = `"function":{"name":"ignored_nested","parameters":{}}`
	const alias = `"name":"mcp__fs__lookup","parameters":{"type":"object"}`
	const projected = `{"function":{"name":"mcp__fs__lookup","parameters":{"type":"object"}},"type":"function"}`
	fixture := requestPrimitiveFixture04384{}
	input, want, choice := `{}`, "", ""
	switch suffix {
	case "tool-kind-missing", "tool-kind-null", "tool-kind-number", "tool-kind-array", "tool-kind-empty", "tool-kind-unknown":
		values := map[string]string{"tool-kind-missing": "", "tool-kind-null": "null", "tool-kind-number": "7", "tool-kind-array": "[]", "tool-kind-empty": `""`, "tool-kind-unknown": `"unsupported"`}
		if value := values[suffix]; value != "" {
			input = `{"type":` + value + `}`
		}
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` is not a supported tool declaration"
	case "tool-kind-nested", "tool-kind-nested-null-type", "tool-kind-nested-number-type", "tool-kind-nested-empty-type", "tool-kind-nested-unknown-type":
		input = `{` + nested + `}`
		values := map[string]string{"tool-kind-nested-null-type": "null", "tool-kind-nested-number-type": "7", "tool-kind-nested-empty-type": `""`, "tool-kind-nested-unknown-type": `"unsupported"`}
		if value := values[suffix]; value != "" {
			input = `{"type":` + value + `,` + nested + `}`
		}
		fixture.reject = suffix == "tool-kind-nested-empty-type" || suffix == "tool-kind-nested-unknown-type"
	case "tool-kind-function-null":
		input = `{"function":null}`
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"
	case "tool-kind-function-name":
		input = `{"type":7,"function":{"name":false,"parameters":{}}}`
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0].function.name` must be a non-empty string"
	case "tool-kind-mcp", "tool-kind-mcp-alias", "tool-kind-mcp-number-type", "tool-kind-mcp-metadata", "tool-kind-mcp-strict", "tool-kind-mcp-null-schema", "tool-kind-mcp-missing-schema", "tool-kind-mcp-invalid-name", "tool-kind-mcp-unknown-type", "tool-kind-mcp-nested", "tool-kind-mcp-empty-segment", "tool-kind-choice":
		input, want = `{`+alias+`}`, projected
		switch suffix {
		case "tool-kind-mcp-alias":
			input = `{"type":null,"name":"mcp__fs__lookup","parametersJsonSchema":{"type":"object"}}`
		case "tool-kind-mcp-number-type":
			input = `{"type":7,` + alias + `}`
		case "tool-kind-mcp-metadata":
			input = `{"description":null,"strict":null,` + alias + `}`
		case "tool-kind-mcp-strict":
			input, fixture.reject = `{"strict":true,`+alias+`}`, true
		case "tool-kind-mcp-null-schema":
			input = `{"name":"mcp__fs__lookup","parameters":null}`
			fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` translates to an invalid declaration: invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"
		case "tool-kind-mcp-missing-schema":
			input = `{"name":"mcp__fs__lookup"}`
			fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` is not a supported tool declaration"
		case "tool-kind-mcp-invalid-name":
			input, fixture.reject = strings.ReplaceAll(input, "mcp__fs__lookup", "mcp__fs.bad"), true
		case "tool-kind-mcp-unknown-type":
			input, fixture.reject = `{"type":"unsupported",`+alias+`}`, true
		case "tool-kind-mcp-nested":
			input = `{` + nested + `,` + alias + `}`
		case "tool-kind-mcp-empty-segment":
			input, want = strings.ReplaceAll(input, "mcp__fs__lookup", "mcp__"), strings.ReplaceAll(want, "mcp__fs__lookup", "mcp__")
		case "tool-kind-choice":
			choice = `,"tool_choice":{"function":{"name":"mcp__fs__lookup"},"type":"function"}`
		}
	case "tool-kind-media", "tool-kind-media-unknown-type":
		input = `{"extra":[{"type":"input_image","path":null}]}`
		if suffix == "tool-kind-media-unknown-type" {
			input, fixture.reject = `{"type":"unsupported","extra":{"type":"input_image","path":null}}`, true
		}
	case "tool-kind-mixed":
		input, want = `{`+nested+`},{"type":"function","name":"mcp__fs__lookup","parameters":{"type":"object"}}`, projected
	case "tool-kind-choice-missing":
		input, choice, fixture.reject = `{`+nested+`}`, `,"tool_choice":{"function":{"name":"ignored_nested"},"type":"function"}`, true
	case "tool-kind-stream":
		input, fixture.geminiChatStream = `{`+nested+`}`, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.reject = fixture.reject || fixture.geminiValidationError != ""
	stream := "false"
	if fixture.geminiChatStream {
		stream = "true"
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + stream + choice + `,"tools":[` + input + `]}`
	fixture.wireBody = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + stream
	if fixture.geminiChatStream {
		fixture.wireBody += `,"stream_options":{"include_usage":true}`
	}
	fixture.wireBody += choice
	if want != "" {
		fixture.wireBody += `,"tools":[` + want + `]`
	}
	fixture.wireBody += "}"
	return fixture, true
}
