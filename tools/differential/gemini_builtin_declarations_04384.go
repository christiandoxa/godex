package main

import "strings"

func geminiBuiltinDeclarationKinds04384() []string {
	return []string{"web_search", "web_search_preview", "web_search_preview_2099", "web_fetch", "url_context", "urlContext", "web_fetch_preview", "web_fetch_preview_2099", "code_interpreter", "code_execution", "codeExecution", "computer", "computer_use", "computerUse", "computer_use_preview", "computer_custom"}
}

func geminiBuiltinDeclarationSuffixes04384() []string {
	var result []string
	for _, kind := range geminiBuiltinDeclarationKinds04384() {
		result = append(result, "builtin-kind-"+kind)
	}
	return append(result, "builtin-key-computerUse", "builtin-key-codeExecution", "builtin-key-urlContext",
		"builtin-function-null", "builtin-function-name", "builtin-function-parameters", "builtin-function-valid",
		"builtin-description-null", "builtin-strict-string", "builtin-strict-true", "builtin-nested-strict-string",
		"builtin-key-unknown-type", "builtin-key-number-type", "builtin-function-key",
		"builtin-mixed", "builtin-count", "builtin-mcp-name", "builtin-web-mcp-name", "builtin-web-options",
		"builtin-choice-required", "builtin-choice-missing", "builtin-stream",
		"builtin-media-recursive", "builtin-media-file-path", "builtin-media-camel", "builtin-media-no-path",
		"builtin-media-wrong-kind", "builtin-media-later", "builtin-media-earlier", "builtin-media-host")
}

func geminiBuiltinDeclarationFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	const function = `{"function":{"description":"synthetic tool","name":"synthetic_lookup","parameters":{"type":"object"}},"type":"function"}`
	fixture := requestPrimitiveFixture04384{}
	input, want, choice := `{"type":"code_execution"}`, "", ""
	switch {
	case strings.HasPrefix(suffix, "builtin-kind-"):
		kind := strings.TrimPrefix(suffix, "builtin-kind-")
		known := false
		for _, supported := range geminiBuiltinDeclarationKinds04384() {
			known = known || kind == supported
		}
		if !known {
			return requestPrimitiveFixture04384{}, false
		}
		input = `{"type":"` + kind + `"}`
	case strings.HasPrefix(suffix, "builtin-key-") && suffix != "builtin-key-unknown-type" && suffix != "builtin-key-number-type":
		key := strings.TrimPrefix(suffix, "builtin-key-")
		if key != "computerUse" && key != "codeExecution" && key != "urlContext" {
			return requestPrimitiveFixture04384{}, false
		}
		input = `{"` + key + `":null}`
	case suffix == "builtin-function-null":
		input = `{"type":"code_execution","function":null}`
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"
	case suffix == "builtin-function-name":
		input = `{"type":"codeExecution","function":{"name":false,"parameters":{}}}`
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0].function.name` must be a non-empty string"
	case suffix == "builtin-function-parameters":
		input = `{"type":"web_search","function":{"name":"synthetic_lookup"}}`
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required"
	case suffix == "builtin-function-valid":
		input = `{"type":"code_execution","function":{"name":"ignored_lookup","parameters":{},"strict":true}}`
	case suffix == "builtin-description-null":
		input, fixture.reject = `{"type":"code_execution","description":null}`, true
	case suffix == "builtin-strict-string":
		input, fixture.reject = `{"type":"code_execution","strict":"false"}`, true
	case suffix == "builtin-strict-true":
		input = `{"type":"code_execution","strict":true}`
	case suffix == "builtin-nested-strict-string":
		input, fixture.reject = `{"type":"code_execution","function":{"name":"ignored_lookup","parameters":{},"strict":"false"}}`, true
	case suffix == "builtin-key-unknown-type":
		input, fixture.reject = `{"type":"unsupported","codeExecution":{}}`, true
	case suffix == "builtin-key-number-type":
		input = `{"type":7,"description":null,"strict":null,"codeExecution":false}`
	case suffix == "builtin-function-key":
		input = `{"type":"function","name":"synthetic_lookup","codeExecution":{}}`
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required"
	case suffix == "builtin-mixed":
		input, want = `{"type":"codeExecution"},`+function+`,{"type":"urlContext"},`+function, function
	case suffix == "builtin-count":
		input, want = strings.Repeat(`{"type":"code_execution"},`, 129)+function, function
	case suffix == "builtin-mcp-name", suffix == "builtin-web-mcp-name":
		kind := "code_execution"
		if suffix == "builtin-web-mcp-name" {
			kind = "web_search"
		}
		input = `{"type":"` + kind + `","name":"mcp__fs__lookup","description":"synthetic tool","parameters":{"type":"object"}}`
		if kind == "code_execution" {
			want = strings.Replace(function, "synthetic_lookup", "mcp__fs__lookup", 1)
		}
	case suffix == "builtin-web-options":
		input = `{"type":"web_search","search_context_size":"high","allowed_domains":["synthetic.invalid"]}`
	case suffix == "builtin-choice-required":
		choice = `,"tool_choice":"required"`
	case suffix == "builtin-choice-missing":
		choice, fixture.reject = `,"tool_choice":{"type":"function","function":{"name":"synthetic_lookup"}}`, true
	case suffix == "builtin-stream":
		input, fixture.geminiChatStream = `{"type":"computerUse"}`, true
	case strings.HasPrefix(suffix, "builtin-media-"):
		input = `{"type":"code_execution","function":null,"extra":[{"nested":{"type":"input_image","path":null}}]}`
		switch suffix {
		case "builtin-media-recursive":
		case "builtin-media-file-path":
			input = strings.ReplaceAll(strings.ReplaceAll(input, "input_image", "input_file"), `"path":null`, `"file_path":false`)
		case "builtin-media-camel":
			input = strings.ReplaceAll(strings.ReplaceAll(input, "input_image", "media"), `"path":null`, `"filePath":{}`)
		case "builtin-media-no-path", "builtin-media-wrong-kind":
			if suffix == "builtin-media-no-path" {
				input = strings.ReplaceAll(input, `"path":null`, `"url":"synthetic.invalid"`)
			} else {
				input = strings.ReplaceAll(input, "input_image", "message")
			}
			fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"
		case "builtin-media-later":
			input = `{"type":"code_execution","function":null},` + input
			fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object"
		case "builtin-media-earlier":
			input += `,{"type":"function","name":"synthetic_lookup"}`
			fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[1].function.parameters` is required"
		case "builtin-media-host":
			input, fixture.reject = strings.TrimSuffix(input, "}")+`,"description":null}`, true
		default:
			return requestPrimitiveFixture04384{}, false
		}
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
