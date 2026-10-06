package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const requestMaxBytes = 16 << 20

type RequestOptions struct {
	Model               string
	StrictTools         bool
	WebSearchMode       string
	BetaBaseURL         string
	SSELookaheadTimeout time.Duration
	StreamIdleTimeout   time.Duration
}

type translatedRequestParts struct {
	thinking         bool
	reasoningFields  map[string]any
	messages         []any
	tools            []any
	toolNames        map[string]bool
	webSearchOptions map[string]any
}

func ResponsesRequest(body []byte, options RequestOptions) ([]byte, error) {
	translated, err := TranslateResponsesRequest(body, options)
	if err != nil {
		return nil, err
	}
	return translated.Body, nil
}

func TranslateResponsesRequest(body []byte, options RequestOptions) (TranslatedRequest, error) {
	return translateResponsesRequestWithHistory(body, options, nil)
}

func translateResponsesRequestWithHistory(body []byte, options RequestOptions, history []any) (TranslatedRequest, error) {
	object, err := parseResponsesObject(body)
	if err != nil {
		return TranslatedRequest{}, err
	}
	parts, err := translateRequestParts(object, options)
	if err != nil {
		return TranslatedRequest{}, err
	}
	parts.messages = repairDeepSeekToolCallAdjacency(mergeDeepSeekHistory(history, parts.messages))
	if parts.thinking {
		parts.messages = normalizeDeepSeekThinkingToolCallMessages(parts.messages)
	}
	metadata, err := deepSeekResponseMetadata(object, parts.thinking)
	if err != nil {
		return TranslatedRequest{}, err
	}
	result, err := buildTranslatedRequest(object, options.Model, parts)
	if err != nil {
		return TranslatedRequest{}, err
	}
	content, err := json.Marshal(result)
	if err != nil {
		return TranslatedRequest{}, err
	}
	return TranslatedRequest{Body: content, ResponseMetadata: metadata}, nil
}

func parseResponsesObject(body []byte) (map[string]any, error) {
	object, err := parseResponsesObjectWithoutValidation(body)
	if err != nil {
		return nil, err
	}
	if err := rejectRequestFields(object); err != nil {
		return nil, err
	}
	return object, nil
}

func parseResponsesObjectWithoutValidation(body []byte) (map[string]any, error) {
	if len(body) > requestMaxBytes {
		return nil, fmt.Errorf("DeepSeek Responses request exceeds %d bytes", requestMaxBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("failed to parse DeepSeek Responses request JSON: %w", err)
	}
	object, ok := root.(map[string]any)
	if !ok {
		return nil, errors.New("DeepSeek Responses request body must be a JSON object")
	}
	return object, nil
}

func translateRequestParts(object map[string]any, options RequestOptions) (translatedRequestParts, error) {
	thinking, reasoningFields, err := deepSeekReasoning(object)
	if err != nil {
		return translatedRequestParts{}, err
	}
	messages, err := deepSeekMessages(object)
	if err != nil {
		return translatedRequestParts{}, err
	}
	tools, toolNames, err := deepSeekTools(object, options.StrictTools)
	if err != nil {
		return translatedRequestParts{}, err
	}
	webSearchOptions, err := deepSeekWebSearchOptions(object, options.WebSearchMode)
	if err != nil {
		return translatedRequestParts{}, err
	}
	return translatedRequestParts{
		thinking: thinking, reasoningFields: reasoningFields,
		messages: messages, tools: tools, toolNames: toolNames,
		webSearchOptions: webSearchOptions,
	}, nil
}

func buildTranslatedRequest(object map[string]any, model string, parts translatedRequestParts) (map[string]any, error) {
	result := map[string]any{
		"model":           deepSeekModel(object, model),
		"messages":        parts.messages,
		deepSeekStreamKey: boolField(object, deepSeekStreamKey),
	}
	if err := copyPrimitiveFields(result, object); err != nil {
		return nil, err
	}
	for key, value := range parts.reasoningFields {
		result[key] = value
	}
	if len(parts.tools) > 0 {
		result["tools"] = parts.tools
	}
	if parts.webSearchOptions != nil {
		result["web_search_options"] = parts.webSearchOptions
	}
	toolChoice, err := deepSeekToolChoice(object, parts.toolNames, parts.thinking)
	if err != nil {
		return nil, err
	}
	if toolChoice != nil {
		result["tool_choice"] = toolChoice
	}
	return applyDeepSeekResponseFormat(result, object, parts.messages)
}

func applyDeepSeekResponseFormat(result, object map[string]any, messages []any) (map[string]any, error) {
	responseFormat, jsonMode, err := deepSeekResponseFormat(object)
	if err != nil {
		return nil, err
	}
	if responseFormat == nil {
		return result, nil
	}
	result["response_format"] = responseFormat
	if jsonMode {
		result["messages"] = ensureJSONInstruction(messages)
	}
	return result, nil
}

func deepSeekModel(object map[string]any, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if model, ok := object["model"].(string); ok && strings.TrimSpace(model) != "" {
		return strings.TrimSpace(model)
	}
	return "deepseek-v4-pro"
}

func rejectRequestFields(object map[string]any) error {
	for _, validator := range []func(map[string]any) error{
		rejectDeprecatedRequestFields,
		rejectUnsupportedRequestFields,
		validatePassiveRequestFields,
		validateExecutionRequestFields,
		validateStreamOptions,
		validateModalities,
		rejectBetaAndContinuationFields,
	} {
		if err := validator(object); err != nil {
			return err
		}
	}
	return nil
}

func rejectDeprecatedRequestFields(object map[string]any) error {
	if _, ok := object["frequency_penalty"]; ok {
		return errors.New("DeepSeek frequency_penalty is deprecated and is not forwarded by Prodex")
	}
	if _, ok := object["presence_penalty"]; ok {
		return errors.New("DeepSeek presence_penalty is deprecated and is not forwarded by Prodex")
	}
	return nil
}

func rejectUnsupportedRequestFields(object map[string]any) error {
	for _, field := range []string{"n", "seed", "service_tier", "prediction", "logit_bias", "functions", "function_call"} {
		if _, ok := object[field]; ok {
			return fmt.Errorf("DeepSeek %s is not supported by this Responses adapter", field)
		}
	}
	if _, ok := object["max_tool_calls"]; ok {
		return errors.New("DeepSeek max_tool_calls is not supported by this Responses adapter")
	}
	return nil
}

func validatePassiveRequestFields(object map[string]any) error {
	if value, ok := object["include"]; ok {
		if _, valid := value.([]any); !valid {
			return errors.New("DeepSeek include must be an array")
		}
	}
	if value, ok := object["store"]; ok {
		if _, valid := value.(bool); !valid {
			return errors.New("DeepSeek store must be a boolean")
		}
	}
	if value, ok := object["text"]; ok {
		if _, valid := value.(map[string]any); !valid {
			return errors.New("DeepSeek text must be an object")
		}
	}
	return nil
}

func validateExecutionRequestFields(object map[string]any) error {
	if value, ok := object["background"]; ok {
		background, valid := value.(bool)
		if !valid {
			return errors.New("DeepSeek background must be a boolean")
		}
		if background {
			return errors.New("DeepSeek background responses are not supported by this Responses adapter")
		}
	}
	if value, ok := object["truncation"]; ok {
		return validateDeepSeekTruncation(value)
	}
	if value, ok := object["parallel_tool_calls"]; ok {
		parallel, valid := value.(bool)
		if !valid {
			return errors.New("DeepSeek parallel_tool_calls must be a boolean")
		}
		if !parallel {
			return errors.New("DeepSeek does not expose a compatible parallel_tool_calls=false control")
		}
	}
	return nil
}

func validateDeepSeekTruncation(value any) error {
	text, valid := value.(string)
	if !valid {
		return errors.New("DeepSeek truncation must be a string")
	}
	if text == "auto" {
		return errors.New("DeepSeek truncation=auto is not supported by this Responses adapter")
	}
	return fmt.Errorf("DeepSeek truncation `%s` is not supported", text)
}

func rejectBetaAndContinuationFields(object map[string]any) error {
	for _, field := range []string{"prefix", "suffix", "prompt"} {
		if _, ok := object[field]; ok {
			return fmt.Errorf("DeepSeek %s completions are outside this Responses adapter", field)
		}
	}
	if value, ok := object["web_search_options"]; ok {
		options, valid := value.(map[string]any)
		if !valid {
			return errors.New("DeepSeek web_search_options must be an object")
		}
		if err := validateDeepSeekWebSearchOptions(options); err != nil {
			return err
		}
	}
	return nil
}

func validateStreamOptions(object map[string]any) error {
	value, ok := object["stream_options"]
	if !ok {
		return nil
	}
	options, valid := value.(map[string]any)
	if !valid {
		return errors.New("DeepSeek stream_options must be an object")
	}
	if !boolField(object, deepSeekStreamKey) {
		return errors.New("DeepSeek stream_options requires stream=true")
	}
	if err := validateStreamOptionKeys(options); err != nil {
		return err
	}
	include, ok := options["include_usage"]
	if !ok {
		return errors.New("DeepSeek streaming adapter requires stream_options.include_usage=true")
	}
	includeUsage, valid := include.(bool)
	if !valid {
		return errors.New("DeepSeek stream_options.include_usage must be a boolean")
	}
	if !includeUsage {
		return errors.New("DeepSeek streaming adapter requires stream_options.include_usage=true")
	}
	return nil
}

func validateStreamOptionKeys(options map[string]any) error {
	for key := range options {
		if key != "include_usage" {
			return fmt.Errorf("DeepSeek stream_options.%s is not supported", key)
		}
	}
	return nil
}

func validateModalities(object map[string]any) error {
	value, ok := object["modalities"]
	if !ok {
		return rejectAudioOutput(object)
	}
	items, valid := value.([]any)
	if !valid {
		return errors.New("DeepSeek modalities must be an array")
	}
	for _, raw := range items {
		text, ok := raw.(string)
		if !ok || text != "text" {
			return errors.New("DeepSeek Responses adapter only supports text modality; audio/image/video modalities are not supported")
		}
	}
	return rejectAudioOutput(object)
}

func rejectAudioOutput(object map[string]any) error {
	if _, audio := object["audio"]; audio {
		return errors.New("DeepSeek Responses adapter does not support audio output")
	}
	return nil
}

func boolField(object map[string]any, key string) bool {
	value, _ := object[key].(bool)
	return value
}
