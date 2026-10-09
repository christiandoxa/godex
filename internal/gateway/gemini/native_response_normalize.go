package gemini

// Code Assist can wrap GenerateContent responses with a trace envelope. Prodex
// uses the trace ID as the response ID while translating the nested payload.
func normalizedGeminiResponse(root map[string]any) map[string]any {
	response, ok := root["response"].(map[string]any)
	traceID, hasTraceID := root["traceId"].(string)
	if !ok || !hasTraceID {
		return root
	}
	normalized := make(map[string]any, len(response)+1)
	for key, value := range response {
		normalized[key] = value
	}
	normalized["responseId"] = traceID
	return normalized
}
