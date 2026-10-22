package main

import "strings"

func geminiChoiceSuffixes04384() []string {
	return []string{"choice-auto", "choice-none", "choice-required", "choice-null", "choice-false", "choice-number", "choice-array",
		"choice-unknown-string", "choice-padded-string", "choice-empty-object", "choice-type-null", "choice-type-unknown",
		"choice-flat", "choice-nested", "choice-top-precedence", "choice-type-fallback", "choice-empty-shadow", "choice-blank-shadow",
		"choice-missing-name", "choice-no-tools", "choice-extra", "choice-namespace-function", "choice-namespace-mcp",
		"choice-namespace-prefix", "choice-namespace-pair-name", "choice-namespace-suffix", "choice-namespace-leading-name",
		"choice-namespace-empty-shadow", "choice-namespace-null-fallback", "choice-namespace-nested", "choice-namespace-raw-leading",
		"choice-name-limit", "choice-name-over", "choice-name-unicode", "choice-name-padded", "choice-thinking-invalid", "choice-stream"}
}

func geminiChoiceFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{}
	name, choice, wireChoice := "synthetic_lookup", `{"type":"function","name":"synthetic_lookup"}`, ""
	noTools, thinking := false, false
	switch suffix {
	case "choice-auto", "choice-none", "choice-required":
		choice = `"` + strings.TrimPrefix(suffix, "choice-") + `"`
		wireChoice = choice
	case "choice-null", "choice-false", "choice-number", "choice-array", "choice-unknown-string", "choice-padded-string", "choice-empty-object", "choice-type-null", "choice-type-unknown":
		values := map[string]string{"choice-null": "null", "choice-false": "false", "choice-number": "7", "choice-array": "[]",
			"choice-unknown-string": `"unsupported"`, "choice-padded-string": `" auto "`, "choice-empty-object": "{}",
			"choice-type-null": `{"type":null,"name":"synthetic_lookup"}`, "choice-type-unknown": `{"type":"other","name":"synthetic_lookup"}`}
		choice, fixture.reject = values[suffix], true
	case "choice-flat":
	case "choice-stream":
		fixture.geminiChatStream = true
	case "choice-nested":
		choice = `{"type":"function","function":{"name":"synthetic_lookup"}}`
	case "choice-top-precedence":
		choice = `{"type":"function","name":"synthetic_lookup","function":{"name":"wrong"}}`
	case "choice-type-fallback":
		choice = `{"type":"function","name":7,"function":{"name":"synthetic_lookup"}}`
	case "choice-empty-shadow", "choice-blank-shadow":
		value := `""`
		if suffix == "choice-blank-shadow" {
			value = `" \t\u2003"`
		}
		choice, fixture.reject = `{"type":"function","name":`+value+`,"function":{"name":"synthetic_lookup"}}`, true
	case "choice-missing-name":
		choice, fixture.reject = `{"type":"function"}`, true
	case "choice-no-tools":
		noTools, fixture.reject = true, true
	case "choice-extra":
		choice = `{"type":"function","name":"synthetic_lookup","extra":{"value":1.0}}`
	case "choice-namespace-function":
		name, choice = "fs--lookup", `{"type":"function","namespace":" \u2003fs\u00a0 ","name":" lookup "}`
	case "choice-namespace-mcp":
		name, choice = "mcp__fs__lookup", `{"type":"mcp_tool","server_label":"fs","name":"lookup"}`
	case "choice-namespace-prefix":
		name, choice = "mcp__fs__lookup", `{"type":"mcp_tool","namespace":"mcp__fs","name":"lookup"}`
	case "choice-namespace-pair-name":
		name, choice = "mcp__fs--group__lookup", `{"type":"mcp_tool","namespace":"fs","name":"group__lookup"}`
	case "choice-namespace-suffix":
		name, choice = "mcp__fs_--lookup", `{"type":"mcp_tool","namespace":"fs_","name":"lookup"}`
	case "choice-namespace-leading-name":
		name, choice = "mcp__fs--_lookup", `{"type":"mcp_tool","namespace":"fs","name":"_lookup"}`
	case "choice-namespace-empty-shadow":
		choice = `{"type":"function","namespace":"","server_label":"ignored","name":"synthetic_lookup"}`
	case "choice-namespace-null-fallback":
		name, choice = "fs--lookup", `{"type":"function","namespace":null,"server_label":"fs","name":"lookup"}`
	case "choice-namespace-nested":
		name, choice = "fs--lookup", `{"type":"function","function":{"namespace":"fs","name":"lookup"}}`
	case "choice-namespace-raw-leading":
		name, choice, fixture.reject = "mcp__fs__lookup", `{"type":"mcp_tool","namespace":" fs ","name":"lookup"}`, true
	case "choice-name-limit", "choice-name-over":
		name = strings.Repeat("n", 128)
		selected := name
		if suffix == "choice-name-over" {
			selected += "x"
			fixture.reject = true
		}
		choice = `{"type":"function","name":"` + selected + `"}`
	case "choice-name-unicode", "choice-name-padded":
		value := "検索"
		if suffix == "choice-name-padded" {
			value = " synthetic_lookup "
		}
		choice, fixture.reject = `{"type":"function","name":"`+value+`"}`, true
	case "choice-thinking-invalid":
		choice, thinking, fixture.geminiOmittedChoice = `{"type":"other","name":false}`, true, `{"name":false,"type":"other"}`
		fixture.geminiMetadata = `{"gemini":{"finishReason":"STOP",` + geminiReasoningChoiceNote04384(fixture.geminiOmittedChoice) + `,"usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}}}`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	if wireChoice == "" {
		wireChoice = `{"function":{"name":"` + name + `"},"type":"function"}`
	}
	stream := "false"
	if fixture.geminiChatStream {
		stream = "true"
	}
	tools := `[{"function":{"description":"synthetic tool","name":"` + name + `","parameters":{"type":"object"}},"type":"function"}]`
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + stream + `,"tool_choice":` + choice
	fixture.wireBody = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview"`
	if thinking {
		fixture.body += `,"reasoning":{"effort":"high"}`
		fixture.wireBody += `,"reasoning_effort":"high"`
	}
	fixture.wireBody += `,"stream":` + stream
	if fixture.geminiChatStream {
		fixture.wireBody += `,"stream_options":{"include_usage":true}`
	}
	if !thinking {
		fixture.wireBody += `,"tool_choice":` + wireChoice
	}
	if !noTools {
		fixture.body += `,"tools":` + tools
		fixture.wireBody += `,"tools":` + tools
	}
	fixture.body += "}"
	fixture.wireBody += "}"
	return fixture, true
}
