package deepseek

import (
	"errors"
	"fmt"
	"strings"
)

func deepSeekReasoning(object map[string]any) (bool, map[string]any, error) {
	effort, err := reasoningEffort(object)
	if err != nil {
		return false, nil, err
	}
	fields := make(map[string]any)
	switch effort {
	case "":
		return false, fields, nil
	case "none", "minimal":
		fields[deepSeekThinkingKey] = map[string]any{"type": "disabled"}
		return false, fields, nil
	case "low", "medium", "high":
		fields[deepSeekReasoningEffortKey] = "high"
		fields[deepSeekThinkingKey] = map[string]any{"type": "enabled"}
		return true, fields, nil
	case "xhigh", "max":
		fields[deepSeekReasoningEffortKey] = "max"
		fields[deepSeekThinkingKey] = map[string]any{"type": "enabled"}
		return true, fields, nil
	default:
		return false, nil, errors.New("DeepSeek reasoning effort is not supported")
	}
}

func reasoningEffort(object map[string]any) (string, error) {
	if value, found := object["reasoning"]; found {
		effort, err := reasoningObjectEffort(value)
		if err != nil || effort != "" {
			return effort, err
		}
	}
	return reasoningEffortField(object[deepSeekReasoningEffortKey])
}

func reasoningObjectEffort(value any) (string, error) {
	mapping, valid := value.(map[string]any)
	if !valid {
		return "", errors.New("DeepSeek reasoning must be an object")
	}
	if err := validateReasoningKeys(mapping); err != nil {
		return "", err
	}
	value, found := mapping["effort"]
	if !found {
		return "", nil
	}
	text, valid := value.(string)
	if !valid {
		return "", errors.New("DeepSeek reasoning.effort must be a string")
	}
	return strings.ToLower(strings.TrimSpace(text)), nil
}

func validateReasoningKeys(mapping map[string]any) error {
	for key := range mapping {
		if key != "effort" {
			return fmt.Errorf("DeepSeek reasoning.%s is not supported by this Responses adapter", key)
		}
	}
	return nil
}

func reasoningEffortField(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	text, valid := value.(string)
	if !valid {
		return "", errors.New("DeepSeek reasoning_effort must be a string")
	}
	return strings.ToLower(strings.TrimSpace(text)), nil
}
