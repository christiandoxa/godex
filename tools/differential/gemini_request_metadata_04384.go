package main

import "strings"

const geminiRequestMetadataNative04384 = `"gemini":{"finishReason":"STOP","usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}}`

func geminiRequestMetadataSuffixes04384() []string {
	return []string{"metadata-empty", "metadata-object", "metadata-provider-empty", "metadata-client", "metadata-cache-key",
		"metadata-cache-blank", "metadata-cache-blank-existing", "metadata-cache-retention", "metadata-cache-retention-empty",
		"metadata-explicit-overrides", "metadata-provider-overrides", "metadata-provider-nested-replacement", "metadata-numbers-unicode",
		"metadata-omission-overrides", "metadata-stream", "metadata-stream-provider-overrides", "metadata-stream-omission", "metadata-fallback",
		"metadata-null", "metadata-array", "metadata-string", "metadata-number", "metadata-bool", "metadata-provider-null",
		"metadata-provider-array", "metadata-client-null", "metadata-client-string", "metadata-cache-null", "metadata-cache-number",
		"metadata-retention-null", "metadata-retention-bool"}
}

func geminiRequestMetadataFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{geminiMetadata: "{" + geminiRequestMetadataNative04384 + "}"}
	switch suffix {
	case "metadata-empty":
		fixture.fields = `"metadata":{}`
	case "metadata-object":
		fixture.fields, fixture.geminiMetadata = `"metadata":{"tag":"synthetic"}`, `{`+geminiRequestMetadataNative04384+`,"tag":"synthetic"}`
	case "metadata-provider-empty":
		fixture.fields = `"metadata":{"gemini":{}}`
	case "metadata-client":
		fixture.fields, fixture.geminiMetadata = `"client_metadata":{"client":"synthetic"}`, `{"client_metadata":{"client":"synthetic"},`+geminiRequestMetadataNative04384+`}`
	case "metadata-cache-key":
		fixture.fields, fixture.geminiMetadata = `"prompt_cache_key":" \u2003synthetic<& \u00a0"`, `{`+geminiRequestMetadataNative04384+`,"prompt_cache_key":"  synthetic<&  "}`
	case "metadata-cache-blank":
		fixture.fields = `"prompt_cache_key":" \t\u2003\u00a0"`
	case "metadata-cache-blank-existing":
		fixture.fields, fixture.geminiMetadata = `"metadata":{"prompt_cache_key":"base"},"prompt_cache_key":" \u2003"`, `{`+geminiRequestMetadataNative04384+`,"prompt_cache_key":"base"}`
	case "metadata-cache-retention":
		fixture.fields, fixture.geminiMetadata = `"prompt_cache_retention":" synthetic-retention "`, `{`+geminiRequestMetadataNative04384+`,"prompt_cache_retention":" synthetic-retention "}`
	case "metadata-cache-retention-empty":
		fixture.fields, fixture.geminiMetadata = `"prompt_cache_retention":""`, `{`+geminiRequestMetadataNative04384+`,"prompt_cache_retention":""}`
	case "metadata-explicit-overrides":
		fixture.fields = `"metadata":{"client_metadata":{"old":true},"prompt_cache_key":"old","prompt_cache_retention":"old","gemini":{"prompt_cache_key":"provider"}},"client_metadata":{"new":true},"prompt_cache_key":"new","prompt_cache_retention":"new"`
		fixture.geminiMetadata = `{"client_metadata":{"new":true},"gemini":{"finishReason":"STOP","prompt_cache_key":"provider","usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}},"prompt_cache_key":"new","prompt_cache_retention":"new"}`
	case "metadata-provider-overrides":
		fixture.fields = `"metadata":{"gemini":{"finishReason":null,"usageMetadata":false}}`
		fixture.geminiMetadata = `{"gemini":{"finishReason":null,"usageMetadata":false}}`
	case "metadata-provider-nested-replacement":
		fixture.fields = `"metadata":{"gemini":{"finishReason":{"source":"request"},"usageMetadata":{"custom":1}}}`
		fixture.geminiMetadata = `{"gemini":{"finishReason":{"source":"request"},"usageMetadata":{"custom":1}}}`
	case "metadata-numbers-unicode", "metadata-fallback":
		fixture.fields = `"metadata":{"decimal":1.0,"exponent":1e3,"negative_zero":-0,"text":"<&/🌍\u2028\u2029","u64":18446744073709551615}`
		fixture.geminiMetadata = `{"decimal":1.0,"exponent":1000.0,` + geminiRequestMetadataNative04384 + `,"negative_zero":-0.0,"text":"<&/🌍  ","u64":18446744073709551615}`
		if suffix == "metadata-fallback" {
			fixture.geminiChatFallback = true
			fixture.geminiMetadata = strings.Replace(fixture.geminiMetadata, geminiRequestMetadataNative04384+",", "", 1)
		}
	case "metadata-omission-overrides", "metadata-stream-omission":
		fixture.fields = `"metadata":{"gemini":{"omitted_tool_choice":{"from":"wrong","reason":"wrong"},"trace":"synthetic"}},"reasoning":{"effort":"high"},"tool_choice":null`
		fixture.wireFields = `"reasoning_effort":"high"`
		fixture.geminiOmittedChoice = "null"
		fixture.geminiMetadata = `{"gemini":{"finishReason":"STOP",` + geminiReasoningChoiceNote04384("null") + `,"trace":"synthetic","usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}}}`
		if suffix == "metadata-stream-omission" {
			fixture.geminiChatStream = true
			fixture.geminiMetadata = `{"gemini":{"finish_reason":"stop",` + geminiReasoningChoiceNote04384("null") + `,"trace":"synthetic"}}`
		}
	case "metadata-stream":
		fixture.fields = `"metadata":{"tag":"synthetic"},"client_metadata":{"decimal":1.0,"text":"<&/🌍\u2028\u2029","u64":18446744073709551615},"prompt_cache_key":" key "`
		fixture.geminiChatStream = true
		fixture.geminiMetadata = `{"client_metadata":{"decimal":1.0,"text":"<&/🌍  ","u64":18446744073709551615},"gemini":{"finish_reason":"stop"},"prompt_cache_key":" key ","tag":"synthetic"}`
	case "metadata-stream-provider-overrides":
		fixture.fields, fixture.geminiMetadata = `"metadata":{"gemini":{"finish_reason":{"source":"request"}}}`, `{"gemini":{"finish_reason":{"source":"request"}}}`
		fixture.geminiChatStream = true
	case "metadata-null", "metadata-array", "metadata-string", "metadata-number", "metadata-bool":
		values := map[string]string{"metadata-null": "null", "metadata-array": "[]", "metadata-string": `"synthetic"`, "metadata-number": "7", "metadata-bool": "false"}
		fixture.fields, fixture.reject = `"metadata":`+values[suffix], true
	case "metadata-provider-null", "metadata-provider-array":
		value := "null"
		if suffix == "metadata-provider-array" {
			value = "[]"
		}
		fixture.fields, fixture.reject = `"metadata":{"gemini":`+value+`}`, true
	case "metadata-client-null", "metadata-client-string":
		value := "null"
		if suffix == "metadata-client-string" {
			value = `"synthetic"`
		}
		fixture.fields, fixture.reject = `"client_metadata":`+value, true
	case "metadata-cache-null", "metadata-cache-number":
		value := "null"
		if suffix == "metadata-cache-number" {
			value = "7"
		}
		fixture.fields, fixture.reject = `"prompt_cache_key":`+value, true
	case "metadata-retention-null", "metadata-retention-bool":
		value := "null"
		if suffix == "metadata-retention-bool" {
			value = "false"
		}
		fixture.fields, fixture.reject = `"prompt_cache_retention":`+value, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	if fixture.geminiChatStream || fixture.wireFields != "" {
		stream := "false"
		if fixture.geminiChatStream {
			stream = "true"
		}
		fixture.body = `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + stream + `,` + fixture.fields + `}`
		fixture.wireBody = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview"`
		if fixture.wireFields != "" {
			fixture.wireBody += "," + fixture.wireFields
		}
		fixture.wireBody += `,"stream":` + stream
		if fixture.geminiChatStream {
			fixture.wireBody += `,"stream_options":{"include_usage":true}`
		}
		fixture.wireBody += "}"
	}
	return fixture, true
}

func geminiRequestMetadataResponse04384(fixture requestPrimitiveFixture04384) string {
	if fixture.geminiChatFallback {
		return `{"error":{"code":"gemini_empty_response","message":"Gemini returned no visible response content."},"id":"gemini_resp_prodex","metadata":` + fixture.geminiMetadata + `,"model":"gemini-2.5-pro","object":"response","output":[],"status":"failed","usage":{}}`
	}
	if fixture.geminiChatStream {
		body := geminiChatSSEFixture043711("resp_deepseek_synthetic", "msg_gemini_synthetic", 1)
		return strings.Replace(body, `"metadata":{"gemini":{"finish_reason":"stop"}}`, `"metadata":`+fixture.geminiMetadata, 1)
	}
	body := strings.Replace(geminiBufferedBodySuccess04384, "response-uuid", "chatcmpl-differential", 1)
	return strings.Replace(body, "{"+geminiRequestMetadataNative04384+"}", fixture.geminiMetadata, 1)
}
