package gemini

import "strings"

func compactSummary(body []byte) string {
	root, valid := geminiSerdeResponseObject04384(body)
	if !valid {
		return ""
	}
	if _, traced := root["traceId"].(string); traced {
		if response, ok := root["response"].(map[string]any); ok {
			root = response
		}
	}
	if choices, _ := root["choices"].([]any); len(choices) != 0 {
		choice, _ := choices[0].(map[string]any)
		message, _ := choice["message"].(map[string]any)
		if text := compactSummaryContent04384(message["content"]); text != "" {
			return text
		}
	}
	var texts []string
	output, _ := root["output"].([]any)
	for _, raw := range output {
		item, _ := raw.(map[string]any)
		if item["type"] == "message" {
			if text := compactSummaryContent04384(item["content"]); text != "" {
				texts = append(texts, text)
			}
		}
	}
	if len(texts) != 0 {
		return strings.Join(texts, "\n")
	}
	if candidates, _ := root["candidates"].([]any); len(candidates) != 0 {
		candidate, _ := candidates[0].(map[string]any)
		content, _ := candidate["content"].(map[string]any)
		parts, _ := content["parts"].([]any)
		for _, raw := range parts {
			part, _ := raw.(map[string]any)
			if part["thought"] != true {
				if text, _ := part["text"].(string); strings.TrimSpace(text) != "" {
					texts = append(texts, strings.TrimSpace(text))
				}
			}
		}
	}
	if len(texts) != 0 {
		return strings.Join(texts, "\n")
	}
	text, _ := root["output_text"].(string)
	return strings.TrimSpace(text)
}

func compactSummaryContent04384(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	parts, _ := value.([]any)
	var texts []string
	for _, raw := range parts {
		part, _ := raw.(map[string]any)
		if text, _ := part["text"].(string); strings.TrimSpace(text) != "" {
			texts = append(texts, strings.TrimSpace(text))
		}
	}
	return strings.Join(texts, "\n")
}
