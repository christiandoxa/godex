package main

import "strings"

func geminiNamespaceDeclarationSuffixes04384() []string {
	return []string{"namespace-declaration-basic", "namespace-declaration-mcp", "namespace-declaration-trim", "namespace-declaration-pair-name",
		"namespace-declaration-description", "namespace-declaration-empty", "namespace-declaration-name-blank", "namespace-declaration-wrong-tools",
		"namespace-declaration-missing-parameters", "namespace-declaration-null-parameters", "namespace-declaration-strict-true",
		"namespace-declaration-mixed-kind", "namespace-declaration-mixed-scalar", "namespace-declaration-mixed-name", "namespace-declaration-name-invalid",
		"namespace-declaration-description-null", "namespace-declaration-strict-string", "namespace-declaration-duplicate", "namespace-declaration-choice",
		"namespace-declaration-stream"}
}

func geminiNamespaceDeclarationFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{}
	namespace, child := "fs", `{"type":"function","name":"read","parameters":{"type":"object"}}`
	wireName, description, choice := "fs--read", "", ""
	switch suffix {
	case "namespace-declaration-basic":
	case "namespace-declaration-mcp":
		namespace, wireName = "mcp__fs", "mcp__fs__read"
	case "namespace-declaration-trim":
		namespace = "  fs  "
		child = strings.Replace(child, `"read"`, `" read "`, 1)
	case "namespace-declaration-pair-name":
		namespace, wireName = "mcp__fs", "mcp__fs--group__read"
		child = strings.Replace(child, `"read"`, `"group__read"`, 1)
	case "namespace-declaration-description":
		child = strings.TrimSuffix(child, "}") + `,"description":" synthetic description "}`
		description = `"description":" synthetic description ",`
	case "namespace-declaration-empty", "namespace-declaration-name-blank", "namespace-declaration-wrong-tools":
		fixture.reject = true
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` is not a supported tool declaration"
		if suffix == "namespace-declaration-name-blank" {
			namespace = "  "
		}
	case "namespace-declaration-missing-parameters", "namespace-declaration-null-parameters":
		fixture.reject = true
		field := "is required"
		if suffix == "namespace-declaration-missing-parameters" {
			child = `{"type":"function","name":"read"}`
		} else {
			child = `{"type":"function","name":"read","parameters":null}`
			field = "must be an object"
		}
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` translates to an invalid declaration: invalid_tool_declaration: Gemini request field `tools[0].function.parameters` " + field
	case "namespace-declaration-strict-true":
		child = strings.TrimSuffix(child, "}") + `,"strict":true}`
		fixture.reject = true
	case "namespace-declaration-mixed-kind", "namespace-declaration-mixed-scalar", "namespace-declaration-mixed-name":
		other := `{"type":"other","name":"other","parameters":{}}`
		if suffix == "namespace-declaration-mixed-scalar" {
			other = "null"
		} else if suffix == "namespace-declaration-mixed-name" {
			other = `{"type":"function","name":"","parameters":{}}`
		}
		child += "," + other
		fixture.reject = true
	case "namespace-declaration-name-invalid":
		namespace, fixture.reject = "invalid.name", true
	case "namespace-declaration-description-null":
		child = strings.TrimSuffix(child, "}") + `,"description":null}`
		fixture.reject = true
	case "namespace-declaration-strict-string":
		child = strings.TrimSuffix(child, "}") + `,"strict":"true"}`
		fixture.reject = true
	case "namespace-declaration-duplicate":
		child += "," + child
	case "namespace-declaration-choice":
		choice = `,"tool_choice":{"function":{"name":"fs--read"},"type":"function"}`
	case "namespace-declaration-stream":
		fixture.geminiChatStream = true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	children := "[" + child + "]"
	if suffix == "namespace-declaration-empty" {
		children = "[]"
	} else if suffix == "namespace-declaration-wrong-tools" {
		children = "null"
	}
	// Namespace strings here contain no quotes or backslashes.
	input := `{"type":"namespace","name":"` + namespace + `","tools":` + children + `}`
	stream := "false"
	if fixture.geminiChatStream {
		stream = "true"
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + stream + choice + `,"tools":[` + input + `]}`
	fixture.wireBody = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + stream
	if fixture.geminiChatStream {
		fixture.wireBody += `,"stream_options":{"include_usage":true}`
	}
	fixture.wireBody += choice + `,"tools":[{"function":{` + description + `"name":"` + wireName + `","parameters":{"type":"object"}` + `},"type":"function"}]}`
	return fixture, true
}
