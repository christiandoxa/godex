package deepseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const deepSeekUserIDError = "DeepSeek user_id must use only letters, numbers, underscores, or dashes and be at most 512 bytes"

func copyPrimitiveFields(target, source map[string]any) error {
	for _, copier := range []func(map[string]any, map[string]any) error{
		copyNumericFields,
		copyMaxTokenField,
		copyLogprobFields,
		copyStopField,
		copyParallelToolField,
		copyUserField,
	} {
		if err := copier(target, source); err != nil {
			return err
		}
	}
	return nil
}

func copyNumericFields(target, source map[string]any) error {
	for _, key := range []string{"temperature", "top_p"} {
		value, ok := source[key]
		if !ok {
			continue
		}
		if !jsonNumber(value) {
			return fmt.Errorf("DeepSeek %s must be a number", key)
		}
		target[key] = value
	}
	return nil
}

func copyMaxTokenField(target, source map[string]any) error {
	for _, key := range []string{"max_completion_tokens", "max_output_tokens", "max_tokens"} {
		value, ok := source[key]
		if !ok {
			continue
		}
		if !positiveInteger(value) {
			return fmt.Errorf("DeepSeek %s must be a positive integer", key)
		}
		target["max_tokens"] = value
		return nil
	}
	return nil
}

func copyLogprobFields(target, source map[string]any) error {
	if value, ok := source[deepSeekLogprobsKey]; ok {
		flag, valid := value.(bool)
		if !valid {
			return errors.New("DeepSeek logprobs must be a boolean")
		}
		target[deepSeekLogprobsKey] = flag
	}
	value, ok := source["top_logprobs"]
	if !ok {
		return nil
	}
	count, valid := integer(value)
	if !valid {
		return errors.New("DeepSeek top_logprobs must be an integer")
	}
	if count > 20 {
		return errors.New("DeepSeek top_logprobs must be <= 20")
	}
	if enabled, _ := source[deepSeekLogprobsKey].(bool); !enabled {
		return errors.New("DeepSeek top_logprobs requires logprobs=true")
	}
	target["top_logprobs"] = value
	return nil
}

func copyStopField(target, source map[string]any) error {
	stop, ok, err := deepSeekStop(source)
	if err != nil {
		return err
	}
	if ok {
		target["stop"] = stop
	}
	return nil
}

func copyParallelToolField(target, source map[string]any) error {
	if value, ok := source["parallel_tool_calls"]; ok {
		target["parallel_tool_calls"] = value
	}
	return nil
}

func copyUserField(target, source map[string]any) error {
	user, err := deepSeekUserID(source)
	if err != nil {
		return err
	}
	if user != "" {
		target["user_id"] = user
	}
	return nil
}

func deepSeekStop(source map[string]any) (any, bool, error) {
	value, ok := firstExistingRequestValue(source, "stop", "stop_sequences", "stopSequences")
	if !ok {
		return nil, false, nil
	}
	switch typed := value.(type) {
	case string:
		return typed, true, nil
	case []any:
		return validateStopArray(typed)
	default:
		return nil, false, errors.New("DeepSeek stop must be a string or array of strings")
	}
}

func validateStopArray(values []any) (any, bool, error) {
	if len(values) > 16 {
		return nil, false, errors.New("DeepSeek supports at most 16 stop sequences")
	}
	for _, item := range values {
		if _, ok := item.(string); !ok {
			return nil, false, errors.New("DeepSeek stop sequences must be strings")
		}
	}
	return values, true, nil
}

func deepSeekUserID(source map[string]any) (string, error) {
	value, found := firstExistingRequestValue(source, "user_id", "user", "safety_identifier")
	if !found {
		return "", nil
	}
	text, valid := value.(string)
	if !valid {
		return "", errors.New("DeepSeek user_id must be a string")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	if !validDeepSeekUserID(text) {
		return "", errors.New(deepSeekUserIDError)
	}
	return text, nil
}

func validDeepSeekUserID(value string) bool {
	if len(value) > 512 {
		return false
	}
	for _, current := range value {
		if current == '_' || current == '-' || current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z' || current >= '0' && current <= '9' {
			continue
		}
		return false
	}
	return true
}

func firstExistingRequestValue(object map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func jsonNumber(value any) bool {
	switch value.(type) {
	case json.Number, float64, float32, int, int64, uint64:
		return true
	default:
		return false
	}
}

func integer(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case float64:
		parsed := int64(typed)
		return parsed, typed == float64(parsed)
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	default:
		return 0, false
	}
}

func positiveInteger(value any) bool {
	parsed, ok := integer(value)
	return ok && parsed > 0
}
