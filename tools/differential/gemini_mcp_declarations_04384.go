package main

import "strings"

func geminiMCPDeclarationSuffixes04384() []string {
	return []string{"mcp-declaration-basic", "mcp-declaration-toolset", "mcp-declaration-none", "mcp-declaration-empty",
		"mcp-declaration-missing-server", "mcp-declaration-server-empty-shadow", "mcp-declaration-server-type-fallback",
		"mcp-declaration-configs", "mcp-declaration-default-disabled", "mcp-declaration-union", "mcp-declaration-sanitize",
		"mcp-declaration-prefixed", "mcp-declaration-no-name-segment", "mcp-declaration-no-server-segment",
		"mcp-declaration-description", "mcp-declaration-description-null", "mcp-declaration-strict-string", "mcp-declaration-set-strict-true",
		"mcp-declaration-single", "mcp-declaration-single-alias", "mcp-declaration-single-missing-schema",
		"mcp-declaration-single-primary-null", "mcp-declaration-single-strict-true", "mcp-declaration-choice", "mcp-declaration-stream"}
}

func geminiMCPDeclarationFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	const generic = `{"additionalProperties":true,"type":"object"}`
	tool := func(name, description, parameters string) string {
		body := `{"function":{`
		if description != "" {
			body += `"description":"` + description + `",`
		}
		return body + `"name":"` + name + `","parameters":` + parameters + `},"type":"function"}`
	}
	fixture := requestPrimitiveFixture04384{}
	input := `{"type":"mcp","server_label":"fs","allowed_tools":["read"]}`
	want := []string{tool("mcp__fs__read", "MCP tool read from fs.", generic)}
	choice, wireChoice := "", ""
	switch suffix {
	case "mcp-declaration-basic":
	case "mcp-declaration-toolset":
		input = strings.Replace(input, `"type":"mcp"`, `"type":"mcp_toolset"`, 1)
	case "mcp-declaration-none":
		input, want = `{"type":"mcp","server_label":"fs"}`, nil
	case "mcp-declaration-empty":
		input, fixture.reject = `{"type":"mcp","server_label":"fs","allowed_tools":[]}`, true
	case "mcp-declaration-missing-server":
		input, fixture.reject = `{"type":"mcp","allowed_tools":["read"]}`, true
	case "mcp-declaration-server-empty-shadow":
		input, want = `{"type":"mcp","mcp_server_name":"","server_label":"fs","allowed_tools":["read"]}`, nil
	case "mcp-declaration-server-type-fallback":
		input = `{"type":"mcp","mcp_server_name":null,"server_label":"fs","allowed_tools":["read"]}`
	case "mcp-declaration-configs":
		input = `{"type":"mcp","server_label":"fs","configs":{"zeta":{},"alpha":{"enabled":false},"beta":{"enabled":true}}}`
		want = []string{tool("mcp__fs__beta", "MCP tool beta from fs.", generic), tool("mcp__fs__zeta", "MCP tool zeta from fs.", generic)}
	case "mcp-declaration-default-disabled":
		input = `{"type":"mcp","server_label":"fs","default_config":{"enabled":false},"configs":{"zeta":{},"alpha":{"enabled":true},"beta":{"enabled":false}}}`
		want = []string{tool("mcp__fs__alpha", "MCP tool alpha from fs.", generic)}
	case "mcp-declaration-union":
		input = `{"type":"mcp","server_label":"fs","allowed_tools":[" zeta ","alpha","alpha"],"configs":{"beta":{},"zeta":{}}}`
		want = []string{tool("mcp__fs__alpha", "MCP tool alpha from fs.", generic), tool("mcp__fs__beta", "MCP tool beta from fs.", generic), tool("mcp__fs__zeta", "MCP tool zeta from fs.", generic)}
	case "mcp-declaration-sanitize":
		input = `{"type":"mcp","server_label":" α git-tools β ","allowed_tools":[" read file "]}`
		want = []string{tool("mcp__git_tools__read_file", "MCP tool read file from  α git-tools β .", generic)}
	case "mcp-declaration-prefixed":
		input = `{"type":"mcp","server_label":"fs","allowed_tools":["mcp__else__read"]}`
		want = []string{tool("mcp__else__read", "MCP tool mcp__else__read from fs.", generic)}
	case "mcp-declaration-no-name-segment":
		input, want = `{"type":"mcp","server_label":"fs","allowed_tools":["___"]}`, nil
	case "mcp-declaration-no-server-segment":
		input, want = `{"type":"mcp","server_label":"___","allowed_tools":["read"]}`, nil
	case "mcp-declaration-description":
		input = `{"type":"mcp","server_label":"fs","allowed_tools":["read"],"description":" synthetic description "}`
		want = []string{tool("mcp__fs__read", " synthetic description ", generic)}
	case "mcp-declaration-description-null":
		input, fixture.reject = `{"type":"mcp","server_label":"fs","allowed_tools":["read"],"description":null}`, true
	case "mcp-declaration-strict-string":
		input, fixture.reject = `{"type":"mcp","server_label":"fs","allowed_tools":["read"],"strict":"true"}`, true
	case "mcp-declaration-set-strict-true":
		input = `{"type":"mcp","server_label":"fs","allowed_tools":["read"],"strict":true}`
	case "mcp-declaration-single", "mcp-declaration-single-alias", "mcp-declaration-single-missing-schema", "mcp-declaration-single-primary-null", "mcp-declaration-single-strict-true":
		input = `{"type":"mcp_tool","name":"mcp__fs__read","parameters":{"type":"object"}}`
		want = []string{tool("mcp__fs__read", "", `{"type":"object"}`)}
		if suffix == "mcp-declaration-single-alias" {
			input = strings.Replace(input, "parameters", "parametersJsonSchema", 1)
		} else if suffix == "mcp-declaration-single-missing-schema" {
			input, fixture.reject = `{"type":"mcp_tool","name":"mcp__fs__read"}`, true
		} else if suffix == "mcp-declaration-single-primary-null" {
			input, fixture.reject = `{"type":"mcp_tool","name":"mcp__fs__read","parameters":null,"schema":{"type":"object"}}`, true
		} else if suffix == "mcp-declaration-single-strict-true" {
			input, fixture.reject = strings.TrimSuffix(input, "}")+`,"strict":true}`, true
		}
	case "mcp-declaration-choice":
		choice = `,"tool_choice":{"type":"mcp_tool","server_label":"fs","name":"read"}`
		wireChoice = `,"tool_choice":{"function":{"name":"mcp__fs__read"},"type":"function"}`
	case "mcp-declaration-stream":
		fixture.geminiChatStream = true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	switch suffix {
	case "mcp-declaration-none", "mcp-declaration-empty", "mcp-declaration-missing-server", "mcp-declaration-server-empty-shadow", "mcp-declaration-no-name-segment", "mcp-declaration-no-server-segment", "mcp-declaration-single-missing-schema":
		fixture.reject = true
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` is not a supported tool declaration"
	case "mcp-declaration-single-primary-null":
		fixture.reject = true
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` translates to an invalid declaration: invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"
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
	fixture.wireBody += wireChoice
	if len(want) > 0 {
		fixture.wireBody += `,"tools":[` + strings.Join(want, ",") + `]`
	}
	fixture.wireBody += "}"
	return fixture, true
}
