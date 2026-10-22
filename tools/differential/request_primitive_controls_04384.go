package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type requestPrimitiveFixture04384 struct {
	fields, limit, wireFields, wirePrefix, wireStop, text, body, wireBody string
	reject                                                                bool
	failureStatus                                                         int
	geminiValidationError                                                 string
	geminiOmittedChoice                                                   string
	geminiMetadata                                                        string
	geminiChatStream                                                      bool
	geminiChatFallback                                                    bool
}

func requestPrimitiveSuffixes04384() []string {
	return []string{"output", "precedence", "all", "shadow-null", "shadow-zero", "shadow-fraction", "u64", "overflow", "parallel-true",
		"numeric-control", "numeric-exponent", "numeric-negative-zero", "numeric-fraction", "numeric-u64", "numeric-unicode",
		"numeric-temperature-null", "numeric-temperature-string", "numeric-top-p-bool", "numeric-top-p-array",
		"logprobs-false", "logprobs-zero", "logprobs-max", "logprobs-null", "logprobs-string", "logprobs-top-null",
		"logprobs-top-negative", "logprobs-top-float", "logprobs-top-over", "logprobs-top-u64", "logprobs-top-missing-flag", "logprobs-top-false-flag",
		"stop-string", "stop-empty-string", "stop-empty-array", "stop-snake", "stop-camel", "stop-precedence", "stop-alias-precedence",
		"stop-shadow-null", "stop-null", "stop-bool", "stop-array-number", "stop-limit", "stop-over"}
}

func deepSeekAdmissionSuffixes04384() []string {
	return []string{"admission-stream-false", "admission-stream-null", "admission-stream-string", "admission-stream-number",
		"admission-instructions-null", "admission-instructions-array", "admission-previous-null", "admission-previous-array", "admission-previous-number",
		"json-valid-unicode", "json-root-null", "json-root-array", "json-root-number", "json-root-boolean", "json-root-string",
		"json-syntax", "json-trailing", "json-surrogate", "json-utf8", "json-overflow", "json-depth",
		"user-id-absent", "user-id-canonical", "user-id-user-alias", "user-id-safety-alias", "user-id-blank", "user-id-trim-limit",
		"user-id-null", "user-id-number", "user-id-over-limit", "user-id-invalid-character",
		"preparation-frequency", "preparation-presence", "preparation-seed", "preparation-n", "preparation-include", "preparation-store",
		"preparation-background", "preparation-truncation", "preparation-text", "preparation-tools", "preparation-tool-choice", "preparation-reasoning", "preparation-web-search"}
}

func geminiPreparationSuffixes04384() []string {
	suffixes := append(geminiCandidateCountSuffixes04384(), geminiBetaFieldSuffixes04384()...)
	suffixes = append(suffixes, geminiInputItemSuffixes04384()...)
	suffixes = append(suffixes, geminiInputKindSuffixes04384()...)
	suffixes = append(suffixes, geminiMessageRoleSuffixes04384()...)
	suffixes = append(suffixes, geminiContentTypeSuffixes04384()...)
	suffixes = append(suffixes, geminiMessageProjectionSuffixes04384()...)
	suffixes = append(suffixes, geminiCallIdentitySuffixes04384()...)
	suffixes = append(suffixes, geminiCallNameSuffixes04384()...)
	suffixes = append(suffixes, geminiToolOutputSuffixes04384()...)
	suffixes = append(suffixes, geminiOrphanOutputSuffixes04384()...)
	suffixes = append(suffixes, geminiFunctionAliasSuffixes04384()...)
	suffixes = append(suffixes, geminiEmptyInputSuffixes04384()...)
	suffixes = append(suffixes, geminiRequestSignatureSuffixes04384()...)
	suffixes = append(suffixes, geminiMCPWireSuffixes04384()...)
	suffixes = append(suffixes, geminiBridgeOutputSuffixes04384()...)
	suffixes = append(suffixes, geminiCustomCallSuffixes04384()...)
	suffixes = append(suffixes, geminiMCPNameSuffixes04384()...)
	suffixes = append(suffixes, geminiShellCallSuffixes04384()...)
	suffixes = append(suffixes, geminiIgnoredMessagesSuffixes04384()...)
	suffixes = append(suffixes, geminiReasoningSuffixes04384()...)
	suffixes = append(suffixes, geminiReasoningChoiceSuffixes04384()...)
	suffixes = append(suffixes, geminiRequestMetadataSuffixes04384()...)
	suffixes = append(suffixes, geminiFormatSuffixes04384()...)
	suffixes = append(suffixes, geminiChoiceSuffixes04384()...)
	suffixes = append(suffixes, geminiFunctionDeclarationSuffixes04384()...)
	suffixes = append(suffixes, geminiMCPDeclarationSuffixes04384()...)
	suffixes = append(suffixes, geminiNamespaceDeclarationSuffixes04384()...)
	return append(suffixes, []string{"preparation-tools", "preparation-reasoning", "preparation-tool-params", "preparation-tool-missing-params",
		"preparation-tool-missing-name", "preparation-tool-description", "preparation-input",
		"tool-priority-stream", "tool-priority-temperature", "tool-priority-tokens", "tool-priority-user", "tool-priority-input", "tool-priority-reasoning", "tool-priority-serde"}...)
}

func requestPrimitiveCase04384(scenario string) (requestPrimitiveFixture04384, bool) {
	if !strings.HasPrefix(scenario, "request-primitive-") && !strings.HasPrefix(scenario, "deepseek-request-primitive-") {
		return requestPrimitiveFixture04384{}, false
	}
	const stops16 = `["s01","s02","s03","s04","s05","s06","s07","s08","s09","s10","s11","s12","s13","s14","s15","s16"]`
	suffix := strings.TrimPrefix(strings.TrimPrefix(scenario, "deepseek-"), "request-primitive-")
	if strings.HasPrefix(suffix, "namespace-declaration-") {
		return geminiNamespaceDeclarationFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "mcp-declaration-") {
		return geminiMCPDeclarationFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "declaration-") {
		return geminiFunctionDeclarationFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "choice-") {
		return geminiChoiceFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "format-") {
		return geminiFormatFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "metadata-") {
		return geminiRequestMetadataFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "reasoning-") {
		if fixture, known := geminiReasoningChoiceFixture04384(suffix); known {
			return fixture, true
		}
		return geminiReasoningFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "ignored-messages-") {
		return geminiIgnoredMessagesFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "shell-call-") {
		return geminiShellCallFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "mcp-name-") {
		return geminiMCPNameFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "custom-call-") {
		return geminiCustomCallFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "bridge-output-") {
		return geminiBridgeOutputFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "mcp-wire-") {
		return geminiMCPWireFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "request-signature-") {
		return geminiRequestSignatureFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "empty-input-") {
		return geminiEmptyInputFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "function-alias-") {
		return geminiFunctionAliasFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "orphan-output-") {
		return geminiOrphanOutputFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "tool-output-") {
		return geminiToolOutputFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "call-name-") {
		return geminiCallNameFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "call-id-") {
		return geminiCallIdentityFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "projection-") {
		return geminiMessageProjectionFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "message-content-") {
		return geminiContentTypeFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "message-role-") {
		return geminiMessageRoleFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "input-kind-") {
		return geminiInputKindFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "input-item-") {
		return geminiInputItemFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "beta-") {
		return geminiBetaFieldFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "candidate-") {
		return geminiCandidateCountFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "tool-priority-") {
		return geminiToolPriorityFixture04384(suffix)
	}
	if strings.HasPrefix(suffix, "user-id-") {
		identity, known := geminiUserIDCase04384("request-" + suffix)
		fixture := requestPrimitiveFixture04384{fields: identity.fields, reject: identity.message != ""}
		if identity.value != "" {
			fixture.wireFields = `"user_id":"` + identity.value + `"`
		}
		return fixture, known
	}
	if strings.HasPrefix(suffix, "json-") {
		body, reject, known := geminiJSONAdmission04384("request-" + suffix)
		fixture := requestPrimitiveFixture04384{body: body, reject: reject}
		if strings.HasPrefix(suffix, "json-root-") {
			fixture.failureStatus = http.StatusInternalServerError
		}
		return fixture, known
	}
	switch suffix {
	case "output":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":11`, limit: "11"}, true
	case "precedence":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":11,"max_tokens":23`, limit: "23"}, true
	case "all":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":11,"max_tokens":23,"max_completion_tokens":31`, limit: "31"}, true
	case "shadow-null":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":null,"max_completion_tokens":31`, reject: true}, true
	case "shadow-zero":
		return requestPrimitiveFixture04384{fields: `"max_tokens":0,"max_completion_tokens":31`, reject: true}, true
	case "shadow-fraction":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":1.0,"max_completion_tokens":31`, reject: true}, true
	case "u64":
		return requestPrimitiveFixture04384{fields: `"max_tokens":18446744073709551615`, limit: "18446744073709551615"}, true
	case "overflow":
		return requestPrimitiveFixture04384{fields: `"max_tokens":18446744073709551616`, reject: true}, true
	case "parallel-true":
		return requestPrimitiveFixture04384{fields: `"parallel_tool_calls":true`}, true
	case "numeric-control":
		return requestPrimitiveFixture04384{fields: `"temperature":0.2,"top_p":0.8`, wireFields: `"temperature":0.2,"top_p":0.8`}, true
	case "numeric-exponent":
		return requestPrimitiveFixture04384{fields: `"temperature":1e3`, wireFields: `"temperature":1000.0`}, true
	case "numeric-negative-zero":
		return requestPrimitiveFixture04384{fields: `"top_p":-0`, wireFields: `"top_p":-0.0`}, true
	case "numeric-fraction":
		return requestPrimitiveFixture04384{fields: `"temperature":0.123456789012345678901234567890`, wireFields: `"temperature":0.12345678901234568`}, true
	case "numeric-u64":
		return requestPrimitiveFixture04384{fields: `"temperature":18446744073709551615`, wireFields: `"temperature":18446744073709551615`}, true
	case "numeric-unicode":
		return requestPrimitiveFixture04384{fields: `"temperature":0.2`, wireFields: `"temperature":0.2`, text: "<>&/🌍" + string([]rune{0x2028, 0x2029})}, true
	case "numeric-temperature-null":
		return requestPrimitiveFixture04384{fields: `"temperature":null`, reject: true}, true
	case "numeric-temperature-string":
		return requestPrimitiveFixture04384{fields: `"temperature":"0.2"`, reject: true}, true
	case "numeric-top-p-bool":
		return requestPrimitiveFixture04384{fields: `"top_p":false`, reject: true}, true
	case "numeric-top-p-array":
		return requestPrimitiveFixture04384{fields: `"top_p":[]`, reject: true}, true
	case "logprobs-false":
		return requestPrimitiveFixture04384{fields: `"logprobs":false`, wirePrefix: `"logprobs":false,`}, true
	case "logprobs-zero":
		return requestPrimitiveFixture04384{fields: `"logprobs":true,"top_logprobs":0`, wirePrefix: `"logprobs":true,`, wireFields: `"top_logprobs":0`}, true
	case "logprobs-max":
		return requestPrimitiveFixture04384{fields: `"logprobs":true,"top_logprobs":20`, wirePrefix: `"logprobs":true,`, wireFields: `"top_logprobs":20`}, true
	case "logprobs-null":
		return requestPrimitiveFixture04384{fields: `"logprobs":null`, reject: true}, true
	case "logprobs-string":
		return requestPrimitiveFixture04384{fields: `"logprobs":"true"`, reject: true}, true
	case "logprobs-top-null":
		return requestPrimitiveFixture04384{fields: `"logprobs":true,"top_logprobs":null`, reject: true}, true
	case "logprobs-top-negative":
		return requestPrimitiveFixture04384{fields: `"logprobs":true,"top_logprobs":-1`, reject: true}, true
	case "logprobs-top-float":
		return requestPrimitiveFixture04384{fields: `"logprobs":true,"top_logprobs":1.0`, reject: true}, true
	case "logprobs-top-over":
		return requestPrimitiveFixture04384{fields: `"logprobs":true,"top_logprobs":21`, reject: true}, true
	case "logprobs-top-u64":
		return requestPrimitiveFixture04384{fields: `"logprobs":true,"top_logprobs":18446744073709551615`, reject: true}, true
	case "logprobs-top-missing-flag":
		return requestPrimitiveFixture04384{fields: `"top_logprobs":2`, reject: true}, true
	case "logprobs-top-false-flag":
		return requestPrimitiveFixture04384{fields: `"logprobs":false,"top_logprobs":0`, reject: true}, true
	case "stop-string":
		return requestPrimitiveFixture04384{fields: `"stop":"STOP"`, wireStop: `"STOP"`}, true
	case "stop-empty-string":
		return requestPrimitiveFixture04384{fields: `"stop":""`, wireStop: `""`}, true
	case "stop-empty-array":
		return requestPrimitiveFixture04384{fields: `"stop":[]`, wireStop: `[]`}, true
	case "stop-snake":
		return requestPrimitiveFixture04384{fields: `"stop_sequences":[" STOP ",""]`, wireStop: `[" STOP ",""]`}, true
	case "stop-camel":
		return requestPrimitiveFixture04384{fields: `"stopSequences":"CAMEL"`, wireStop: `"CAMEL"`}, true
	case "stop-precedence":
		return requestPrimitiveFixture04384{fields: `"stop":"PRIMARY","stop_sequences":null,"stopSequences":false`, wireStop: `"PRIMARY"`}, true
	case "stop-alias-precedence":
		return requestPrimitiveFixture04384{fields: `"stop_sequences":"SECOND","stopSequences":"THIRD"`, wireStop: `"SECOND"`}, true
	case "stop-shadow-null":
		return requestPrimitiveFixture04384{fields: `"stop":null,"stop_sequences":"SECOND"`, reject: true}, true
	case "stop-null":
		return requestPrimitiveFixture04384{fields: `"stop":null`, reject: true}, true
	case "stop-bool":
		return requestPrimitiveFixture04384{fields: `"stop":false`, reject: true}, true
	case "stop-array-number":
		return requestPrimitiveFixture04384{fields: `"stop":["STOP",1]`, reject: true}, true
	case "stop-limit":
		return requestPrimitiveFixture04384{fields: `"stop":` + stops16, wireStop: stops16}, true
	case "stop-over":
		return requestPrimitiveFixture04384{fields: `"stop":` + strings.TrimSuffix(stops16, "]") + `,"s17"]`, reject: true}, true
	case "admission-stream-false":
		return requestPrimitiveFixture04384{fields: `"stream":false`}, true
	case "admission-stream-null":
		return requestPrimitiveFixture04384{fields: `"stream":null`, reject: true}, true
	case "admission-stream-string":
		return requestPrimitiveFixture04384{fields: `"stream":"true"`, reject: true}, true
	case "admission-stream-number":
		return requestPrimitiveFixture04384{fields: `"stream":7`, reject: true}, true
	case "admission-instructions-null":
		return requestPrimitiveFixture04384{fields: `"instructions":null`, reject: true}, true
	case "admission-instructions-array":
		return requestPrimitiveFixture04384{fields: `"instructions":[]`, reject: true}, true
	case "admission-previous-null":
		return requestPrimitiveFixture04384{fields: `"previous_response_id":null`, reject: true}, true
	case "admission-previous-array":
		return requestPrimitiveFixture04384{fields: `"previous_response_id":[]`, reject: true}, true
	case "admission-previous-number":
		return requestPrimitiveFixture04384{fields: `"previous_response_id":7`, reject: true}, true
	case "preparation-frequency":
		return requestPrimitiveFixture04384{fields: `"frequency_penalty":0.2`, reject: true}, true
	case "preparation-presence":
		return requestPrimitiveFixture04384{fields: `"presence_penalty":0.2`, reject: true}, true
	case "preparation-seed":
		return requestPrimitiveFixture04384{fields: `"seed":7`, reject: true}, true
	case "preparation-n":
		return requestPrimitiveFixture04384{fields: `"n":2`, reject: true}, true
	case "preparation-include":
		return requestPrimitiveFixture04384{fields: `"include":null`, reject: true}, true
	case "preparation-store":
		return requestPrimitiveFixture04384{fields: `"store":null`, reject: true}, true
	case "preparation-background":
		return requestPrimitiveFixture04384{fields: `"background":true`, reject: true}, true
	case "preparation-truncation":
		return requestPrimitiveFixture04384{fields: `"truncation":"auto"`, reject: true}, true
	case "preparation-text":
		return requestPrimitiveFixture04384{fields: `"text":null`, reject: true}, true
	case "preparation-tools":
		return requestPrimitiveFixture04384{fields: `"tools":"opaque"`, reject: true,
			geminiValidationError: "invalid_tool_declaration: Gemini request field `tools` must be an array"}, true
	case "preparation-tool-choice":
		return requestPrimitiveFixture04384{fields: `"tool_choice":{"type":"function","name":"missing_tool"}`, reject: true}, true
	case "preparation-reasoning":
		return requestPrimitiveFixture04384{fields: `"reasoning":"opaque"`, reject: true}, true
	case "preparation-web-search":
		return requestPrimitiveFixture04384{fields: `"web_search_options":{"search_context_size":"huge"}`, reject: true}, true
	case "preparation-tool-params":
		return requestPrimitiveFixture04384{fields: `"tools":[{"type":"function","function":{"name":"synthetic_lookup","parameters":false}}]`, reject: true,
			geminiValidationError: "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"}, true
	case "preparation-tool-missing-params":
		return requestPrimitiveFixture04384{fields: `"tools":[{"type":"function","name":"synthetic_lookup"}]`, reject: true,
			geminiValidationError: "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required"}, true
	case "preparation-tool-missing-name":
		return requestPrimitiveFixture04384{fields: `"tools":[{"type":"function","function":{"parameters":{}}}]`, reject: true,
			geminiValidationError: "invalid_tool_declaration: Gemini request field `tools[0].function.name` must be a non-empty string"}, true
	case "preparation-tool-description":
		return requestPrimitiveFixture04384{fields: `"tools":[{"type":"function","name":"synthetic_lookup","parameters":{},"description":7}]`, reject: true,
			geminiValidationError: "invalid_tool_declaration: Gemini request field `tools[0].function.description` must be a string"}, true
	case "preparation-input":
		return requestPrimitiveFixture04384{body: `{"input":7,"model":"gemini-3.1-pro-preview"}`, reject: true}, true
	}
	return requestPrimitiveFixture04384{}, false
}

func requestPrimitiveInput04384(scenario, model string) []byte {
	fixture, _ := requestPrimitiveCase04384(scenario)
	if fixture.body != "" {
		return []byte(strings.Replace(fixture.body, "gemini-3.1-pro-preview", model, 1))
	}
	text := "same request"
	if fixture.text != "" {
		text = fixture.text
	}
	encoded, _ := json.Marshal(text)
	body := `{"model":"` + model + `","input":` + string(encoded)
	if fixture.fields != "" {
		body += "," + fixture.fields
	}
	return []byte(body + `}`)
}

func requestPrimitiveWire04384(scenario, model string) string {
	fixture, _ := requestPrimitiveCase04384(scenario)
	if fixture.wireBody != "" {
		return strings.Replace(fixture.wireBody, "gemini-3.1-pro-preview", model, 1)
	}
	body := "{" + fixture.wirePrefix
	if fixture.limit != "" {
		body += `"max_tokens":` + fixture.limit + ","
	}
	text := "same request"
	if fixture.text != "" {
		text = fixture.text
	}
	// Fixture text has no quotes, backslashes or control bytes; expected wire escaping stays independent.
	body += `"messages":[{"content":"` + text + `","role":"user"}],"model":"` + model + `"`
	if fixture.wireStop != "" {
		body += `,"stop":` + fixture.wireStop
	}
	body += `,"stream":false`
	if fixture.wireFields != "" {
		body += "," + fixture.wireFields
	}
	return body + "}"
}

func geminiPrimitiveExpected04384(scenario string) geminiCandidateResult {
	fixture, _ := requestPrimitiveCase04384(scenario)
	if fixture.reject {
		result := geminiUserIDExpected04384("request-user-id-number")
		if fixture.geminiValidationError != "" {
			result.Client = exchange{Status: 400, Headers: http.Header{"Content-Type": {"application/json"},
				"Cache-Control": {"no-store"}, "X-Content-Type-Options": {"nosniff"}},
				Body: `{"error":{"code":"invalid_request_error","message":"` + fixture.geminiValidationError + `"}}`}
			result.Stdout = result.Client.Body + "\n"
		}
		if fixture.failureStatus == http.StatusInternalServerError {
			result.Client = exchange{Status: 500, Headers: make(http.Header)}
			result.Stdout = "\n"
		}
		return result
	}
	result := geminiUserIDExpected04384("request-user-id-absent")
	if fixture.geminiOmittedChoice != "" {
		result.Client.Body = strings.Replace(result.Client.Body, `"finishReason":"STOP",`, `"finishReason":"STOP",`+geminiReasoningChoiceNote04384(fixture.geminiOmittedChoice)+`,`, 1)
		result.Stdout = result.Client.Body + "\n"
	}
	if fixture.geminiChatFallback {
		result.Client.Body = `{"error":{"code":"gemini_empty_response","message":"Gemini returned no visible response content."},"id":"gemini_resp_prodex","metadata":{"gemini":{` + geminiReasoningChoiceNote04384(fixture.geminiOmittedChoice) + `}},"model":"gemini-2.5-pro","object":"response","output":[],"status":"failed","usage":{}}`
		result.Stdout = result.Client.Body + "\n"
	}
	if fixture.geminiChatStream {
		result.Client.Body = geminiReasoningChoiceStreamFixture04384(fixture.geminiOmittedChoice)
		result.Client.Headers.Set("Content-Type", "text/event-stream; charset=utf-8")
		result.Stdout = result.Client.Body + "\n"
	}
	if fixture.geminiMetadata != "" {
		result.Client.Body = geminiRequestMetadataResponse04384(fixture)
		result.Stdout = result.Client.Body + "\n"
	}
	result.Upstream[0].RawBody = requestPrimitiveWire04384(scenario, "gemini-3.1-pro-preview")
	_ = json.Unmarshal([]byte(result.Upstream[0].RawBody), &result.Upstream[0].Body)
	return result
}

func deepSeekPrimitiveExpected04384(scenario string) productRun {
	fixture, _ := requestPrimitiveCase04384(scenario)
	result := productRun{Client: exchange{Status: 200, Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}},
		Body: `{"created_at":1,"id":"chatcmpl-differential","metadata":{"deepseek":{"finish_reason":"stop"}},"model":"deepseek-v4-pro","object":"response","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`}}
	if fixture.reject {
		result.ExitStatus = 2
		result.Client = exchange{Status: 502, Headers: http.Header{"Content-Type": {"text/plain; charset=utf-8"}}, Body: "upstream request failed"}
		if fixture.failureStatus == http.StatusInternalServerError {
			result.Client = exchange{Status: 500, Headers: make(http.Header)}
		}
	} else {
		result.Upstream = []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: requestPrimitiveWire04384(scenario, "deepseek-v4-pro")}}
	}
	result.Stdout = result.Client.Body + "\n"
	return result
}

func deepSeekPrimitiveInvariants04384(scenario scenarioResult) []string {
	if len(scenario.Runs) != 2 {
		return []string{scenario.Name + ".run_count"}
	}
	want := deepSeekPrimitiveExpected04384(scenario.Name)
	var failures []string
	for _, run := range scenario.Runs {
		if run.ClientError != "" || len(run.StateIntegrity) != 0 || run.Retries != 0 || run.Cancelled || run.ExitStatus != want.ExitStatus ||
			run.Client.Status != want.Client.Status || run.Client.Body != want.Client.Body || run.Client.Headers.Get("Content-Type") != want.Client.Headers.Get("Content-Type") ||
			want.Client.Status == 500 && len(run.Client.Headers["Content-Type"]) != 0 ||
			run.Stdout != want.Stdout || len(run.Upstream) != len(want.Upstream) {
			failures = append(failures, scenario.Name+"."+run.Name+".exact_admission_or_response")
			continue
		}
		for index, request := range run.Upstream {
			expected := want.Upstream[index]
			if request.Method != expected.Method || request.Path != expected.Path || !request.AuthOK || request.KeySlot != expected.KeySlot || request.Body != expected.Body {
				failures = append(failures, scenario.Name+"."+run.Name+".exact_original_request")
			}
		}
	}
	return failures
}

func geminiPrimitiveInvariant04384(scenario string, result geminiCandidateResult) error {
	fixture, _ := requestPrimitiveCase04384(scenario)
	if fixture.geminiChatStream {
		return geminiReasoningChoiceStreamInvariant04384(scenario, result)
	}
	if fixture.geminiValidationError != "" && (result.Client.Headers.Get("Cache-Control") != "no-store" || result.Client.Headers.Get("X-Content-Type-Options") != "nosniff") {
		return errors.New("invalid Gemini request lost terminal response headers")
	}
	if fixture.failureStatus == 500 && len(result.Client.Headers["Content-Type"]) != 0 {
		return errors.New("nonobject root invented a content type")
	}
	if !geminiExactRequestResult04384(result, geminiPrimitiveExpected04384(scenario)) {
		return errors.New("Gemini primitive controls lost exact limit, admission, request or response")
	}
	return nil
}

func requestPrimitiveNegativeControl04384() bool {
	for _, suffix := range append(append(requestPrimitiveSuffixes04384(), deepSeekAdmissionSuffixes04384()...), geminiPreparationSuffixes04384()...) {
		scenario := "request-primitive-" + suffix
		gemini := geminiPrimitiveExpected04384(scenario)
		deepseek := deepSeekPrimitiveExpected04384(scenario)
		if len(geminiCompare(scenario, gemini, gemini)) != 0 || len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: "deepseek-" + scenario, Runs: []productRun{deepseek, deepseek}})) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.Body = `{}` },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("Content-Type", "application/x-corrupt-fixture") },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("Content-Type", "") },
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Stdout = "changed" },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"changed"} },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, geminiCapturedRequest{}) },
		} {
			changed := gemini
			changed.Client.Headers = gemini.Client.Headers.Clone()
			changed.Upstream = append([]geminiCapturedRequest(nil), gemini.Upstream...)
			mutate(&changed)
			if len(geminiCompare(scenario, changed, changed)) == 0 {
				return false
			}
		}
		for _, mutate := range []func(*productRun){
			func(r *productRun) { r.Client.Status++ },
			func(r *productRun) { r.Client.Body = `{}` },
			func(r *productRun) { r.Client.Headers.Set("Content-Type", "application/x-corrupt-fixture") },
			func(r *productRun) { r.Client.Headers.Set("Content-Type", "") },
			func(r *productRun) { r.ExitStatus++ },
			func(r *productRun) { r.Stdout = "changed" },
			func(r *productRun) { r.StateIntegrity = []string{"changed"} },
			func(r *productRun) { r.ClientError = "missing exchange" },
			func(r *productRun) { r.Upstream = append(r.Upstream, upstreamRequest{}) },
			func(r *productRun) { r.Retries++ },
		} {
			changed := deepseek
			changed.Client.Headers = deepseek.Client.Headers.Clone()
			changed.Upstream = append([]upstreamRequest(nil), deepseek.Upstream...)
			mutate(&changed)
			if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: "deepseek-" + scenario, Runs: []productRun{changed, changed}})) == 0 {
				return false
			}
		}
		if len(gemini.Upstream) != 0 {
			for _, field := range []string{"wire", "auth", "slot"} {
				changedGemini, changedDeepSeek := geminiPrimitiveExpected04384(scenario), deepSeekPrimitiveExpected04384(scenario)
				switch field {
				case "wire":
					changedGemini.Upstream[0].RawBody, changedDeepSeek.Upstream[0].Body = `{}`, `{}`
				case "auth":
					changedGemini.Upstream[0].Auth, changedDeepSeek.Upstream[0].AuthOK = false, false
				case "slot":
					changedGemini.Upstream[0].Slot, changedDeepSeek.Upstream[0].KeySlot = "secondary", "secondary"
				}
				if len(geminiCompare(scenario, changedGemini, changedGemini)) == 0 ||
					len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: "deepseek-" + scenario, Runs: []productRun{changedDeepSeek, changedDeepSeek}})) == 0 {
					return false
				}
			}
		}
		if gemini.Client.Status == 400 {
			for _, header := range []string{"Cache-Control", "X-Content-Type-Options"} {
				changed := gemini
				changed.Client.Headers = gemini.Client.Headers.Clone()
				changed.Client.Headers.Del(header)
				if len(geminiCompare(scenario, changed, changed)) == 0 {
					return false
				}
			}
		}
	}
	return true
}
