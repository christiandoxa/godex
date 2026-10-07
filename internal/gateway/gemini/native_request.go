package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func geminiGenerateContentRequest(chat, original map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	contents := make([]any, 0)
	var systemParts []any

	messages, _ := chat["messages"].([]any)
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		switch role {
		case "system", "developer":
			for _, text := range geminiMessageTexts(message["content"]) {
				systemParts = append(systemParts, map[string]any{"text": text})
			}
		case "assistant":
			parts := make([]any, 0)
			for _, text := range geminiMessageTexts(message["content"]) {
				parts = append(parts, map[string]any{"text": text})
			}
			calls, _ := message["tool_calls"].([]any)
			for _, rawCall := range calls {
				call, _ := rawCall.(map[string]any)
				function, _ := call["function"].(map[string]any)
				name, _ := function["name"].(string)
				args := geminiJSONArguments(function["arguments"])
				item := map[string]any{"name": name, "args": args}
				if id, _ := call["id"].(string); strings.TrimSpace(id) != "" {
					item["id"] = id
				}
				if extra, ok := call["extra_content"].(map[string]any); ok {
					if google, ok := extra["google"].(map[string]any); ok {
						if sig, _ := google["thought_signature"].(string); sig != "" {
							item["thoughtSignature"] = sig
						}
					}
				}
				parts = append(parts, map[string]any{"functionCall": item})
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "model", "parts": parts})
			}
		case "tool":
			callID, _ := message["tool_call_id"].(string)
			name, _ := message["name"].(string)
			if name == "" {
				name = callID
			}
			response := geminiToolResponseValue(message["content"])
			item := map[string]any{"name": name, "response": response}
			if callID != "" {
				item["id"] = callID
			}
			contents = append(contents, map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": item}}})
		default:
			parts := make([]any, 0)
			for _, text := range geminiMessageTexts(message["content"]) {
				parts = append(parts, map[string]any{"text": text})
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]any{"role": "user", "parts": parts})
			}
		}
	}
	if len(systemParts) > 0 {
		result["systemInstruction"] = map[string]any{"parts": systemParts}
	}
	if len(contents) > 0 {
		result["contents"] = contents
	}

	tools := geminiNativeTools(chat)
	if web := geminiWebSearchOptions(original["tools"]); web != nil {
		tools = append(tools, map[string]any{"googleSearch": map[string]any{}})
	}
	if len(tools) > 0 {
		result["tools"] = tools
	}
	if config := geminiNativeToolConfig(chat["tool_choice"]); config != nil {
		result["toolConfig"] = config
	}
	generation, err := geminiNativeGenerationConfig(original, chat)
	if err != nil {
		return nil, err
	}
	if len(generation) > 0 {
		result["generationConfig"] = generation
	}
	for _, pair := range [][2]string{{"safety_settings", "safetySettings"}, {"safetySettings", "safetySettings"}, {"cached_content", "cachedContent"}, {"cachedContent", "cachedContent"}, {"labels", "labels"}} {
		if value, ok := original[pair[0]]; ok {
			if _, exists := result[pair[1]]; !exists && value != nil {
				result[pair[1]] = value
			}
		}
	}
	return result, nil
}

func geminiMessageTexts(value any) []string {
	switch value := value.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []any:
		result := make([]string, 0, len(value))
		for _, raw := range value {
			switch part := raw.(type) {
			case string:
				if part != "" {
					result = append(result, part)
				}
			case map[string]any:
				if text, _ := part["text"].(string); text != "" {
					result = append(result, text)
				} else if text, _ := part["content"].(string); text != "" {
					result = append(result, text)
				}
			}
		}
		return result
	}
	return nil
}

func geminiJSONArguments(value any) any {
	switch value := value.(type) {
	case string:
		var parsed any
		if json.Unmarshal([]byte(value), &parsed) == nil {
			return parsed
		}
		return map[string]any{}
	case nil:
		return map[string]any{}
	default:
		return value
	}
}

func geminiToolResponseValue(value any) any {
	text := ""
	switch value := value.(type) {
	case string:
		text = value
	case []any:
		if values := geminiMessageTexts(value); len(values) > 0 {
			text = strings.Join(values, "")
		}
	default:
		return map[string]any{"output": value}
	}
	var parsed any
	if json.Unmarshal([]byte(text), &parsed) == nil {
		return parsed
	}
	return map[string]any{"output": text}
}

func geminiNativeTools(chat map[string]any) []any {
	items, _ := chat["tools"].([]any)
	declarations := make([]any, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		function, _ := item["function"].(map[string]any)
		if function == nil {
			continue
		}
		declaration := make(map[string]any)
		for _, key := range []string{"name", "description", "parameters"} {
			if value, ok := function[key]; ok {
				declaration[key] = value
			}
		}
		if _, ok := declaration["name"]; ok {
			declarations = append(declarations, declaration)
		}
	}
	if len(declarations) == 0 {
		return nil
	}
	return []any{map[string]any{"functionDeclarations": declarations}}
}

func geminiNativeToolConfig(value any) map[string]any {
	if value == nil {
		return nil
	}
	config := map[string]any{}
	switch value := value.(type) {
	case string:
		switch value {
		case "none":
			config["mode"] = "NONE"
		case "required":
			config["mode"] = "ANY"
		case "auto":
			return nil
		}
	case map[string]any:
		function, _ := value["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name == "" {
			name, _ = value["name"].(string)
		}
		if name != "" {
			config["mode"] = "ANY"
			config["allowedFunctionNames"] = []any{name}
		}
	}
	if len(config) == 0 {
		return nil
	}
	return map[string]any{"functionCallingConfig": config}
}

func geminiNativeGenerationConfig(original, chat map[string]any) (map[string]any, error) {
	config := make(map[string]any)
	aliases := []struct{ source, canonical string }{
		{"temperature", "temperature"},
		{"top_p", "topP"}, {"topP", "topP"},
		{"top_k", "topK"}, {"topK", "topK"},
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
	if effort, _ := chat["reasoning_effort"].(string); effort != "" && effort != "none" {
		level := strings.ToUpper(effort)
		if level == "XHIGH" || level == "MAX" {
			level = "HIGH"
		}
		config["thinkingConfig"] = map[string]any{"includeThoughts": true, "thinkingLevel": level}
	}
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

func geminiCandidateCount(request map[string]any) (int64, bool, error) {
	var value any
	var found bool
	if current, ok := request["candidate_count"]; ok {
		value, found = current, true
	}
	if current, ok := request["candidateCount"]; ok {
		if found && current != nil && value != nil && fmt.Sprint(current) != fmt.Sprint(value) {
			return 0, false, errors.New("Gemini invalid_candidate_count: candidate_count and candidateCount conflict")
		}
		value, found = current, true
	}
	if !found || value == nil {
		return 0, false, nil
	}
	number, ok := value.(json.Number)
	if ok {
		parsed, err := number.Int64()
		if err == nil && parsed == 1 {
			return 1, true, nil
		}
	}
	if number, ok := value.(float64); ok && number == 1 && number == float64(int64(number)) {
		return 1, true, nil
	}
	if integer, ok := value.(int); ok && integer == 1 {
		return 1, true, nil
	}
	return 0, false, errors.New("Gemini invalid_candidate_count: candidate_count must be omitted, null, or 1")
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
