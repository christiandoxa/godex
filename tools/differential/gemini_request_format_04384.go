package main

import "strings"

const geminiFormatDegradedReason04384 = "Gemini OpenAI-compatible response_format supports json_object but not native JSON Schema enforcement"

func geminiFormatSuffixes04384() []string {
	return []string{"format-text", "format-json", "format-json-object", "format-json-schema", "format-structured", "format-alias",
		"format-top-text-precedence", "format-top-invalid-shadow", "format-null", "format-array", "format-missing-type", "format-unknown",
		"format-upper", "format-blank-type", "format-type-null", "format-type-number", "format-extra-keys", "format-text-null", "format-text-other",
		"format-guidance-user", "format-guidance-system", "format-guidance-developer", "format-guidance-assistant", "format-guidance-tool",
		"format-guidance-metadata", "format-guidance-split", "format-stream", "format-fallback", "format-note-overrides", "format-thinking"}
}

func geminiFormatFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{fields: `"response_format":{"type":"json_object"}`}
	messages, input := `{"content":"same request","role":"user"}`, `"input":"same request"`
	jsonMode, guidance, kind := true, true, ""
	switch suffix {
	case "format-text":
		fixture.fields, jsonMode = `"response_format":{"type":"text"}`, false
	case "format-json":
		fixture.fields = `"response_format":{"type":"json"}`
	case "format-json-object":
	case "format-json-schema", "format-stream", "format-fallback", "format-note-overrides", "format-thinking":
		kind, fixture.fields = "json_schema", `"response_format":{"type":"json_schema"}`
		fixture.geminiChatStream, fixture.geminiChatFallback = suffix == "format-stream", suffix == "format-fallback"
		if suffix == "format-note-overrides" {
			fixture.fields += `,"metadata":{"gemini":{"degraded_response_format":{"from":"wrong","to":"wrong","reason":"wrong"},"trace":"synthetic"}}`
		}
		if suffix == "format-thinking" {
			fixture.fields += `,"reasoning":{"effort":"high"},"tool_choice":null`
			fixture.geminiOmittedChoice = "null"
		}
	case "format-structured":
		kind, fixture.fields = "structured_output", `"response_format":{"type":"structured_output","schema":false}`
	case "format-alias":
		fixture.fields = `"text":{"format":{"type":"json"}}`
	case "format-top-text-precedence":
		fixture.fields, jsonMode = `"response_format":{"type":"text"},"text":{"format":{"type":"json_schema"}}`, false
	case "format-top-invalid-shadow":
		fixture.fields, fixture.reject = `"response_format":null,"text":{"format":{"type":"json_object"}}`, true
	case "format-null", "format-array", "format-missing-type", "format-unknown", "format-upper", "format-blank-type", "format-type-null", "format-type-number":
		values := map[string]string{"format-null": "null", "format-array": "[]", "format-missing-type": "{}", "format-unknown": `{"type":"xml"}`,
			"format-upper": `{"type":"JSON_OBJECT"}`, "format-blank-type": `{"type":" "}`, "format-type-null": `{"type":null}`, "format-type-number": `{"type":7}`}
		fixture.fields, fixture.reject = `"response_format":`+values[suffix], true
	case "format-extra-keys":
		fixture.fields = `"response_format":{"type":"json_object","extra":false,"schema":[1.0,null]}`
	case "format-text-null":
		fixture.fields, jsonMode = `"text":null`, false
	case "format-text-other":
		fixture.fields, jsonMode = `"text":{"verbosity":"synthetic","unused":[null]}`, false
	case "format-guidance-user":
		input, messages, guidance = `"input":"jSoN"`, `{"content":"jSoN","role":"user"}`, false
	case "format-guidance-system":
		input, messages, guidance = input+`,"instructions":"Return JSON"`, `{"content":"Return JSON","role":"system"},`+messages, false
	case "format-guidance-developer":
		input, messages, guidance = `"input":[{"type":"message","role":"developer","content":"Return JSON"}]`, `{"content":"Return JSON","role":"system"}`, false
	case "format-guidance-assistant":
		input, messages = `"input":[{"type":"message","role":"assistant","content":"JSON"}]`, `{"content":"JSON","role":"assistant"}`
	case "format-guidance-tool":
		input = `"input":[{"type":"function_call","call_id":"synthetic-call","name":"synthetic_lookup","arguments":"{}"},{"type":"function_call_output","call_id":"synthetic-call","output":"JSON"}]`
		messages = `{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"{}","name":"synthetic_lookup"},"id":"synthetic-call","type":"function"}]},{"content":"JSON","role":"tool","tool_call_id":"synthetic-call"}`
	case "format-guidance-metadata":
		fixture.fields += `,"metadata":{"note":"JSON"}`
		fixture.geminiMetadata = `{` + geminiRequestMetadataNative04384 + `,"note":"JSON"}`
	case "format-guidance-split":
		input = `"input":[{"type":"message","role":"user","content":[{"text":"JS"},{"text":"ON"}]}]`
		messages = `{"content":"JS\nON","role":"user"}`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	stream := "false"
	if fixture.geminiChatStream {
		stream = "true"
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview",` + input + `,"stream":` + stream + `,` + fixture.fields + `}`
	if jsonMode && guidance {
		messages = `{"content":"Respond with valid JSON only.","role":"system"},` + messages
	}
	fixture.wireBody = `{"messages":[` + messages + `],"model":"gemini-3.1-pro-preview"`
	if suffix == "format-thinking" {
		fixture.wireBody += `,"reasoning_effort":"high"`
	}
	if jsonMode {
		fixture.wireBody += `,"response_format":{"type":"json_object"}`
	}
	fixture.wireBody += `,"stream":` + stream
	if fixture.geminiChatStream {
		fixture.wireBody += `,"stream_options":{"include_usage":true}`
	}
	fixture.wireBody += "}"
	if kind != "" {
		note := `"degraded_response_format":{"from":"` + kind + `","reason":"` + geminiFormatDegradedReason04384 + `","to":"json_object"}`
		provider := `"finishReason":"STOP","usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}`
		if fixture.geminiChatStream {
			provider = `"finish_reason":"stop"`
		} else if fixture.geminiChatFallback {
			provider = ""
		}
		fixture.geminiMetadata = `{"gemini":{` + note
		if provider != "" {
			fixture.geminiMetadata += "," + provider
		}
		if suffix == "format-note-overrides" {
			fixture.geminiMetadata = strings.Replace(fixture.geminiMetadata, `,"usageMetadata"`, `,"trace":"synthetic","usageMetadata"`, 1)
		}
		if suffix == "format-thinking" {
			fixture.geminiMetadata = strings.Replace(fixture.geminiMetadata, `,"usageMetadata"`, `,`+geminiReasoningChoiceNote04384("null")+`,"usageMetadata"`, 1)
		}
		fixture.geminiMetadata += "}}"
	}
	return fixture, true
}
