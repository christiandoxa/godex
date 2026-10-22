package main

func geminiSearchDeclarationSuffixes04384() []string {
	return []string{"search-declaration-basic", "search-declaration-name", "search-declaration-description", "search-declaration-blank-description",
		"search-declaration-description-null", "search-declaration-strict-false", "search-declaration-strict-true", "search-declaration-strict-string",
		"search-declaration-missing-parameters", "search-declaration-null-parameters", "search-declaration-array-parameters", "search-declaration-alias",
		"search-declaration-empty-parameters", "search-declaration-nested", "search-declaration-nested-missing-parameters", "search-declaration-nested-null",
		"search-declaration-duplicate", "search-declaration-conflict", "search-declaration-mcp-name", "search-declaration-mcp-alias",
		"search-declaration-choice", "search-declaration-choice-wrong", "search-declaration-builtin-key", "search-declaration-media", "search-declaration-stream",
		"search-declaration-function-media", "search-declaration-function-media-null"}
}

func geminiSearchDeclarationFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	const schema = `{"additionalProperties":false,"properties":{"query":{"type":"string"}},"required":["query"],"type":"object"}`
	declaration := func(name, description, parameters string) string {
		body := `{"function":{`
		if description != "" {
			body += `"description":"` + description + `",`
		}
		body += `"name":"` + name + `"`
		if parameters != "" {
			body += `,"parameters":` + parameters
		}
		return body + `},"type":"function"}`
	}
	fixture := requestPrimitiveFixture04384{}
	fields, parameters, description, choice := "", `,"parameters":`+schema, "", ""
	want := declaration("tool_search", "", schema)
	switch suffix {
	case "search-declaration-basic":
	case "search-declaration-name":
		fields = `,"name":"client_supplied","namespace":"ignored"`
	case "search-declaration-description":
		fields, description = `,"description":" synthetic search "`, " synthetic search "
	case "search-declaration-blank-description":
		fields = `,"description":" \t\u2003"`
	case "search-declaration-description-null":
		fields, fixture.reject = `,"description":null`, true
	case "search-declaration-strict-false":
		fields = `,"strict":false`
	case "search-declaration-strict-true":
		fields = `,"strict":true`
	case "search-declaration-strict-string":
		fields, fixture.reject = `,"strict":"false"`, true
	case "search-declaration-missing-parameters", "search-declaration-alias":
		parameters = ""
		if suffix == "search-declaration-alias" {
			parameters = `,"parametersJsonSchema":` + schema
		}
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` translates to an invalid declaration: invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required"
	case "search-declaration-null-parameters", "search-declaration-array-parameters":
		parameters = `,"parameters":null`
		if suffix == "search-declaration-array-parameters" {
			parameters = `,"parameters":[]`
		}
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` translates to an invalid declaration: invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"
	case "search-declaration-empty-parameters":
		parameters, want = `,"parameters":{}`, declaration("tool_search", "", `{}`)
	case "search-declaration-nested", "search-declaration-nested-missing-parameters":
		fields = `,"function":{"name":"ignored_nested","parameters":{}}`
		if suffix == "search-declaration-nested-missing-parameters" {
			parameters, want = "", declaration("tool_search", "", "")
		}
	case "search-declaration-nested-null":
		fields = `,"function":null`
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"
	case "search-declaration-duplicate":
	case "search-declaration-conflict":
		fixture.reject = true
	case "search-declaration-mcp-name", "search-declaration-mcp-alias":
		fields = `,"name":"mcp__fs__lookup"`
		want = declaration("mcp__fs__lookup", "", schema) + "," + want
		if suffix == "search-declaration-mcp-alias" {
			parameters = `,"schema":` + schema
			fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` translates to an invalid declaration: invalid_tool_declaration: Gemini request field `tools[1].function.parameters` is required"
		}
	case "search-declaration-choice", "search-declaration-choice-wrong":
		name := "tool_search"
		if suffix == "search-declaration-choice-wrong" {
			name, fields, fixture.reject = "client_supplied", `,"name":"client_supplied"`, true
		}
		choice = `,"tool_choice":{"function":{"name":"` + name + `"},"type":"function"}`
	case "search-declaration-builtin-key", "search-declaration-media":
		parameters, want = "", declaration("tool_search", "", "")
		fields = `,"codeExecution":null`
		if suffix == "search-declaration-media" {
			fields = `,"extra":{"type":"input_image","path":null}`
		}
	case "search-declaration-stream":
		fixture.geminiChatStream = true
	case "search-declaration-function-media", "search-declaration-function-media-null":
		fields = `,"name":"synthetic_lookup","extra":{"type":"input_image","path":null}`
		parameters, want = "", declaration("synthetic_lookup", "", "")
		if suffix == "search-declaration-function-media-null" {
			parameters, fixture.reject = `,"parameters":null`, true
		}
	default:
		return requestPrimitiveFixture04384{}, false
	}
	if description != "" {
		want = declaration("tool_search", description, schema)
	}
	fixture.reject = fixture.reject || fixture.geminiValidationError != ""
	input := `{"type":"tool_search"` + fields + parameters + `}`
	if suffix == "search-declaration-function-media" || suffix == "search-declaration-function-media-null" {
		input = `{"type":"function"` + fields + parameters + `}`
	}
	if suffix == "search-declaration-duplicate" {
		input += "," + input
	} else if suffix == "search-declaration-conflict" {
		input += `,{"type":"tool_search","parameters":{"type":"object","properties":{}}}`
	}
	stream := "false"
	if fixture.geminiChatStream {
		stream = "true"
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + stream + choice + `,"tools":[` + input + `]}`
	fixture.wireBody = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + stream
	if fixture.geminiChatStream {
		fixture.wireBody += `,"stream_options":{"include_usage":true}`
	}
	fixture.wireBody += choice + `,"tools":[` + want + `]}`
	return fixture, true
}
