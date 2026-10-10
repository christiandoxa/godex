package gemini

import (
	"encoding/json"
	"errors"
	"strings"
)

func geminiNativeGenerationConfig(original, chat map[string]any) (map[string]any, error) {
	config := make(map[string]any)
	aliases := []struct{ source, canonical string }{
		{"temperature", "temperature"},
		{"top_p", "topP"}, {"topP", "topP"},
		{"top_k", "topK"}, {"topK", "topK"},
		{"seed", "seed"},
		{"presence_penalty", "presencePenalty"}, {"presencePenalty", "presencePenalty"},
		{"frequency_penalty", "frequencyPenalty"}, {"frequencyPenalty", "frequencyPenalty"},
		{"max_tokens", "maxOutputTokens"}, {"max_output_tokens", "maxOutputTokens"},
		{"response_schema", "responseSchema"}, {"responseSchema", "responseSchema"},
	}
	for _, alias := range aliases {
		if value, exists := original[alias.source]; exists && value != nil {
			config[alias.canonical] = value
		}
	}
	if stop, exists := original["stop"]; exists && stop != nil {
		config["stopSequences"] = stop
	} else if stop, exists := original["stop_sequences"]; exists && stop != nil {
		config["stopSequences"] = stop
	}
	if candidate, exists, err := geminiCandidateCount(original); err != nil {
		return nil, err
	} else if exists {
		config["candidateCount"] = candidate
	}
	model, _ := chat["model"].(string)
	effort, _ := chat["reasoning_effort"].(string)
	config["thinkingConfig"] = geminiThinkingConfig(original, model, effort)
	format, _ := chat["response_format"].(map[string]any)
	if format != nil {
		if kind, _ := format["type"].(string); kind == "json_object" {
			config["responseMimeType"] = "application/json"
		}
	}
	if schema := geminiResponseSchema(original); schema != nil {
		config["responseJsonSchema"] = schema
		config["responseMimeType"] = "application/json"
	}
	return config, nil
}

func geminiThinkingConfig(original map[string]any, model, effort string) map[string]any {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "none" || effort == "minimal" {
		return map[string]any{"includeThoughts": false, "thinkingBudget": 0}
	}
	if strings.Contains(model, "gemini-3") || strings.Contains(model, "gemma-3") || strings.Contains(model, "gemma-4") {
		level := "HIGH"
		if effort == "low" {
			level = "LOW"
		} else if effort == "medium" {
			level = "MEDIUM"
		}
		return map[string]any{"includeThoughts": true, "thinkingLevel": level}
	}
	budget := int64(8192)
	if value, ok := original["thinking_budget"]; ok {
		budget = geminiThinkingBudget(value, budget)
	} else if value, ok := original["thinkingBudget"]; ok {
		budget = geminiThinkingBudget(value, budget)
	} else if effort == "low" {
		budget = 1024
	} else if effort == "xhigh" {
		budget = 24576
	}
	return map[string]any{"includeThoughts": true, "thinkingBudget": budget}
}

func geminiThinkingBudget(value any, fallback int64) int64 {
	switch typed := value.(type) {
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed >= 0 {
			return parsed
		}
	case int:
		if typed >= 0 {
			return int64(typed)
		}
	case int64:
		if typed >= 0 {
			return typed
		}
	case float64:
		if typed >= 0 && typed == float64(int64(typed)) {
			return int64(typed)
		}
	}
	return fallback
}

func geminiCandidateCount(request map[string]any) (int64, bool, error) {
	snake, snakeFound := request["candidate_count"]
	camel, camelFound := request["candidateCount"]
	snakeActive := snakeFound && snake != nil
	camelActive := camelFound && camel != nil

	if snakeActive && camelActive && !geminiCandidateValuesEqual(snake, camel) {
		return 0, false, errors.New("invalid_candidate_count: Gemini request fields `candidate_count` and `candidateCount` conflict")
	}
	if snakeActive && !geminiCandidateCountIsOne(snake) {
		return 0, false, errors.New("invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1")
	}
	if camelActive && !geminiCandidateCountIsOne(camel) {
		return 0, false, errors.New("invalid_candidate_count: Gemini request field `candidateCount` must be omitted, null, or 1")
	}
	if snakeActive || camelActive {
		return 1, true, nil
	}
	return 0, false, nil
}

func geminiCandidateValuesEqual(left, right any) bool {
	switch left := left.(type) {
	case json.Number:
		right, ok := right.(json.Number)
		return ok && left.String() == right.String()
	case int:
		right, ok := right.(int)
		return ok && left == right
	case int64:
		right, ok := right.(int64)
		return ok && left == right
	default:
		return false
	}
}

func geminiCandidateCountIsOne(value any) bool {
	switch value := value.(type) {
	case json.Number:
		return value.String() == "1"
	case int:
		return value == 1
	case int64:
		return value == 1
	default:
		return false
	}
}

func geminiResponseSchema(request map[string]any) any {
	for _, key := range []string{"responseSchema", "response_schema"} {
		if value, ok := request[key]; ok && value != nil {
			return value
		}
	}
	for _, key := range []string{"response_format", "text"} {
		format, _ := request[key].(map[string]any)
		if key == "text" {
			format, _ = format["format"].(map[string]any)
		}
		if format == nil {
			continue
		}
		if schema, ok := format["schema"]; ok {
			return schema
		}
		if nested, ok := format["json_schema"].(map[string]any); ok {
			if schema, ok := nested["schema"]; ok {
				return schema
			}
		}
	}
	return nil
}
