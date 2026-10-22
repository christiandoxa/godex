package main

func geminiRequestSignatureSuffixes04384() []string {
	return []string{"request-signature-none", "request-signature-gemini", "request-signature-snake", "request-signature-camel", "request-signature-provider", "request-signature-extra",
		"request-signature-precedence", "request-signature-null-fallback", "request-signature-empty-fallback", "request-signature-type-fallback", "request-signature-blank", "request-signature-control-space",
		"request-signature-nested", "request-signature-orphan", "request-signature-custom", "request-signature-mcp", "request-signature-duplicate", "request-signature-duplicate-none",
		"request-signature-invalid-id", "request-signature-stream", "request-signature-unicode"}
}

func geminiRequestSignatureFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fields, wireSignature := `,"gemini_thought_signature":"synthetic-signature"`, `"extra_content":{"google":{"thought_signature":"synthetic-signature"}},`
	wireLegacy := ""
	kind, arguments, wireArguments := "function_call", `"arguments":"{}"`, `{}`
	stream, duplicate, orphan := false, "", false
	fixture := requestPrimitiveFixture04384{}
	switch suffix {
	case "request-signature-none":
		fields, wireSignature = "", ""
	case "request-signature-gemini":
	case "request-signature-snake":
		fields = `,"thought_signature":"synthetic-signature"`
	case "request-signature-camel":
		fields = `,"thoughtSignature":"synthetic-signature"`
	case "request-signature-provider":
		fields = `,"provider_specific_fields":{"thought_signature":"synthetic-signature","ignored":true}`
	case "request-signature-extra":
		fields = `,"extra_content":{"google":{"thought_signature":"synthetic-signature","ignored":true},"ignored":true}`
	case "request-signature-precedence":
		fields += `,"thought_signature":"ignored","thoughtSignature":"ignored","provider_specific_fields":{"thought_signature":"ignored"},"extra_content":{"google":{"thought_signature":"ignored"}}`
	case "request-signature-null-fallback":
		fields = `,"gemini_thought_signature":null,"thought_signature":"synthetic-signature"`
	case "request-signature-empty-fallback":
		fields = `,"gemini_thought_signature":"","thought_signature":"","thoughtSignature":"synthetic-signature"`
	case "request-signature-type-fallback":
		fields = `,"gemini_thought_signature":false,"thought_signature":7,"thoughtSignature":[],"provider_specific_fields":{"thought_signature":"synthetic-signature"}`
	case "request-signature-blank":
		fields, wireSignature, wireLegacy = `,"gemini_thought_signature":" \t\u2003","thought_signature":"ignored"`, "", `,"gemini_thought_signature":" \t "`
	case "request-signature-control-space":
		fields, wireSignature, wireLegacy = `,"gemini_thought_signature":"\u001c","thought_signature":"ignored"`, "", `,"gemini_thought_signature":"\u001c"`
	case "request-signature-nested":
		fields, wireSignature = `,"function":{"thought_signature":"ignored"}`, ""
	case "request-signature-orphan":
		orphan = true
	case "request-signature-custom":
		kind, arguments, wireArguments = "custom_tool_call", `"input":"synthetic-input"`, `{\"input\":\"synthetic-input\"}`
	case "request-signature-mcp":
		kind = "mcp_call"
	case "request-signature-duplicate", "request-signature-duplicate-none":
		duplicate = `,{"type":"function_call","call_id":"synthetic-call","name":"synthetic_lookup","arguments":"{}","thought_signature":"ignored"}`
		if suffix == "request-signature-duplicate-none" {
			fields, wireSignature = "", ""
		}
	case "request-signature-invalid-id":
		fixture.reject = true
	case "request-signature-stream":
		stream = true
	case "request-signature-unicode":
		fields, wireSignature = `,"thought_signature":" <&🌍 "`, `"extra_content":{"google":{"thought_signature":" <&🌍 "}},`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	identity := `"call_id":"synthetic-call"`
	if fixture.reject {
		identity = `"call_id":null`
	}
	output := `,{"type":"function_call_output","call_id":"synthetic-call","output":"synthetic-result"}`
	if orphan {
		output = ""
	}
	streamInput, streamWire := "false", "false"
	if stream {
		streamInput, streamWire = "true", `true,"stream_options":{"include_usage":true}`
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","stream":` + streamInput + `,"input":[{"role":"user","content":"same request"},{"type":"` + kind + `",` + identity + `,"name":"synthetic_lookup",` + arguments + fields + `}` + duplicate + output + `]}`
	messages := `{"content":"same request","role":"user"}`
	if !orphan {
		messages += `,{"content":"","role":"assistant","tool_calls":[{` + wireSignature + `"function":{"arguments":"` + wireArguments + `","name":"synthetic_lookup"}` + wireLegacy + `,"id":"synthetic-call","type":"function"}]},{"content":"synthetic-result","role":"tool","tool_call_id":"synthetic-call"}`
	}
	fixture.wireBody = `{"messages":[` + messages + `],"model":"gemini-3.1-pro-preview","stream":` + streamWire + `}`
	return fixture, true
}
