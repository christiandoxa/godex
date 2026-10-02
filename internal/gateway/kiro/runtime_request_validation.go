package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func decodeRuntimeObject(body []byte, surface string) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, newKiroRequestError("invalid_json", fmt.Sprintf("Kiro %s request body must be valid JSON", surface))
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, newKiroRequestError("invalid_json", fmt.Sprintf("Kiro %s request body must be valid JSON", surface))
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, newKiroRequestError("invalid_request_body", fmt.Sprintf("Kiro %s request body must be a JSON object", surface))
	}
	return object, nil
}

func validateKiroChatControls(object map[string]any) error {
	if err := validateKiroChatShapeControls(object); err != nil {
		return err
	}
	if err := validateKiroChatNumericControls(object); err != nil {
		return err
	}
	if err := validateKiroChatToolControls(object); err != nil {
		return err
	}
	if field, value, found := firstKiroTokenLimit(object); found {
		return kiroTokenLimitError(field, value)
	}
	delete(object, "user")
	return validateKiroResponsesControls(object, false)
}

func validateKiroChatShapeControls(object map[string]any) error {
	if value, found := object[kiroFieldResponseFormat]; found && value != nil && !runtimeTextFormat(value) {
		return newKiroRequestError(kiroErrorUnsupportedResponseFormat, "Kiro provider only supports chat response_format type 'text' right now")
	}
	delete(object, kiroFieldResponseFormat)
	if value, found := object["n"]; found && value != nil && !runtimePositiveIntegerEquals(value, 1) {
		return newKiroRequestError("unsupported_choice_count", "Kiro provider only supports chat completion parameter n=1 right now")
	}
	delete(object, "n")
	if value, found := object["stop"]; found && kiroStopRequested(value) {
		return newKiroRequestError("unsupported_stop", "Kiro provider does not support chat stop sequences right now")
	}
	delete(object, "stop")
	return nil
}

func validateKiroChatNumericControls(object map[string]any) error {
	controls := []struct {
		key      string
		expected float64
		code     string
		message  string
	}{
		{kiroFieldTemperature, 1, "unsupported_temperature", "Kiro provider does not support non-default chat temperature right now"},
		{kiroFieldTopP, 1, "unsupported_top_p", "Kiro provider does not support non-default chat top_p right now"},
		{"presence_penalty", 0, "unsupported_presence_penalty", "Kiro provider does not support non-default chat presence_penalty right now"},
		{"frequency_penalty", 0, "unsupported_frequency_penalty", "Kiro provider does not support non-default chat frequency_penalty right now"},
	}
	for _, control := range controls {
		if err := validateKiroChatNumber(object, control.key, control.expected, control.code, control.message); err != nil {
			return err
		}
	}
	if value, found := object["seed"]; found && value != nil {
		return newKiroRequestError("unsupported_seed", "Kiro provider does not support chat seed right now")
	}
	delete(object, "seed")
	return nil
}

func validateKiroChatNumber(object map[string]any, key string, expected float64, code, message string) error {
	value, found := object[key]
	if found && value != nil && !runtimeNumberEquals(value, expected) {
		return newKiroRequestError(code, message)
	}
	delete(object, key)
	return nil
}

func validateKiroChatToolControls(object map[string]any) error {
	if value, found := object[kiroFieldParallelToolCalls]; found && value != nil {
		enabled, ok := value.(bool)
		if !ok || !enabled {
			return newKiroRequestError("unsupported_parallel_tool_calls", "Kiro provider does not support chat parallel_tool_calls right now")
		}
	}
	delete(object, kiroFieldParallelToolCalls)
	return nil
}

func validateKiroResponsesControls(object map[string]any, allowTokenLimit bool) error {
	if err := validateKiroResponsesGenerationControls(object, allowTokenLimit); err != nil {
		return err
	}
	if err := validateKiroResponsesLogprobControls(object); err != nil {
		return err
	}
	if err := validateKiroResponseFormats(object); err != nil {
		return err
	}
	if err := validateKiroResponsesToolControls(object); err != nil {
		return err
	}
	if err := validateKiroReasoning(object); err != nil {
		return err
	}
	clearKiroResponsesControls(object)
	return nil
}

func validateKiroResponsesGenerationControls(object map[string]any, allowTokenLimit bool) error {
	if field := firstPresentNonNull(object, kiroFieldTemperature, kiroFieldTopP, "seed"); field != "" {
		return newKiroRequestError("unsupported_generation_control", fmt.Sprintf("Kiro ACP does not expose the %s control", field))
	}
	if !allowTokenLimit {
		if field, value, found := firstKiroTokenLimit(object); found {
			return kiroTokenLimitError(field, value)
		}
	}
	for _, key := range []string{"stop", "stop_sequences", "stopSequences"} {
		if value, found := object[key]; found && kiroStopRequested(value) {
			return newKiroRequestError("unsupported_stop", "Kiro ACP does not expose stop-sequence controls")
		}
	}
	return nil
}

func validateKiroResponsesLogprobControls(object map[string]any) error {
	if value, found := object["logprobs"]; found && value != nil {
		enabled, ok := value.(bool)
		if !ok {
			return newKiroRequestError("invalid_logprobs", "Kiro logprobs must be a boolean")
		}
		if enabled {
			return newKiroRequestError("unsupported_logprobs", "Kiro ACP does not expose log probabilities")
		}
	}
	if value, found := object["top_logprobs"]; found && value != nil {
		return newKiroRequestError("unsupported_logprobs", "Kiro ACP does not expose top_logprobs")
	}
	return nil
}

func validateKiroResponsesToolControls(object map[string]any) error {
	if value, found := object["tool_choice"]; found && value != nil && !runtimeAutoChoice(value) {
		return newKiroRequestError("unsupported_tool_choice", "Kiro ACP owns tool selection and cannot honor tool_choice")
	}
	if value, found := object["tools"]; found && value != nil && !runtimeEmptyArray(value) {
		return newKiroRequestError("unsupported_tools", "Kiro ACP owns its tool inventory and cannot execute external tools")
	}
	if _, found := object["web_search_options"]; found {
		return newKiroRequestError("unsupported_web_search_options", "Kiro ACP owns web search and cannot honor web_search_options")
	}
	if value, found := object[kiroFieldParallelToolCalls]; found && value != nil {
		enabled, ok := value.(bool)
		if !ok || !enabled {
			return newKiroRequestError("unsupported_parallel_tool_calls", "Kiro ACP does not expose parallel_tool_calls=false")
		}
	}
	return nil
}

func clearKiroResponsesControls(object map[string]any) {
	for _, key := range []string{
		kiroFieldTemperature, kiroFieldTopP, "seed", "stop", "stop_sequences", "stopSequences",
		"logprobs", "top_logprobs", kiroFieldResponseFormat, "tool_choice", "tools", "functions",
		"web_search_options", kiroFieldParallelToolCalls,
	} {
		delete(object, key)
	}
}

func validateKiroResponseFormats(object map[string]any) error {
	if value, found := object[kiroFieldResponseFormat]; found && value != nil && !runtimeTextFormat(value) {
		return newKiroRequestError(kiroErrorUnsupportedResponseFormat, "Kiro ACP supports only text response format")
	}
	text, ok := object["text"].(map[string]any)
	if !ok {
		return nil
	}
	if format, found := text["format"]; found && format != nil && !runtimeTextFormat(format) {
		return newKiroRequestError(kiroErrorUnsupportedResponseFormat, "Kiro ACP supports only text response format")
	}
	return nil
}

func validateKiroReasoning(object map[string]any) error {
	if raw, found := object["reasoning"]; found && raw != nil {
		if err := validateKiroReasoningObject(raw); err != nil {
			return err
		}
	}
	if effort, found := object["reasoning_effort"]; found && effort != nil {
		return validateKiroReasoningEffort(effort)
	}
	return nil
}

func validateKiroReasoningObject(raw any) error {
	reasoning, ok := raw.(map[string]any)
	if !ok {
		return newKiroRequestError(kiroErrorInvalidRequest, "Kiro reasoning must be an object")
	}
	for key := range reasoning {
		if key != "effort" {
			return newKiroRequestError(kiroErrorInvalidRequest, "Kiro reasoning."+key+" is unsupported")
		}
	}
	if effort, found := reasoning["effort"]; found {
		return validateKiroReasoningEffort(effort)
	}
	return nil
}

func validateKiroReasoningEffort(value any) error {
	text, ok := value.(string)
	if !ok {
		return newKiroRequestError(kiroErrorInvalidRequest, "Kiro reasoning effort must be a string")
	}
	switch text {
	case "none", "low", "medium", "high", "xhigh", "max":
		return nil
	default:
		return newKiroRequestError("unsupported_reasoning_effort", fmt.Sprintf("Kiro ACP does not support reasoning effort `%s`", text))
	}
}

func firstPresentNonNull(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, found := object[key]; found && value != nil {
			return key
		}
	}
	return ""
}

func firstKiroTokenLimit(object map[string]any) (string, any, bool) {
	for _, key := range []string{"max_output_tokens", "max_tokens", "max_completion_tokens"} {
		if value, found := object[key]; found && value != nil {
			return key, value, true
		}
	}
	return "", nil, false
}

func kiroTokenLimitError(field string, value any) error {
	if !runtimePositiveInteger(value) {
		return newKiroRequestError("unsupported_token_limit", fmt.Sprintf("Kiro %s must be a positive integer", field))
	}
	return newKiroRequestError("unsupported_token_limit", fmt.Sprintf("Kiro ACP does not expose the %s control", field))
}

func kiroStopRequested(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return typed != ""
	case []any:
		return len(typed) > 0
	default:
		return true
	}
}

func runtimePositiveInteger(value any) bool {
	number, ok := value.(json.Number)
	if !ok || strings.ContainsAny(number.String(), ".eE") {
		return false
	}
	parsed, err := number.Int64()
	return err == nil && parsed > 0
}

func runtimePositiveIntegerEquals(value any, expected int64) bool {
	number, ok := value.(json.Number)
	if !ok || strings.ContainsAny(number.String(), ".eE") {
		return false
	}
	parsed, err := number.Int64()
	return err == nil && parsed == expected
}

func runtimeNumberEquals(value any, expected float64) bool {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return err == nil && parsed == expected
	case float64:
		return typed == expected
	default:
		return false
	}
}
