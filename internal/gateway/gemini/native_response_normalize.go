package gemini

// Code Assist can wrap GenerateContent responses with a trace envelope.
// Tagged Prodex 0.437.13 unwraps a nested response object regardless of
// whether traceId was supplied; only a JSON string traceId overrides its
// original responseId. Never inherit unrelated fields from the outer object
// or mutate the provider-owned source map.
func normalizedGeminiResponse(root map[string]any) map[string]any {
	response, ok := root["response"].(map[string]any)
	if !ok {
		return root
	}
	normalized := make(map[string]any, len(response)+1)
	for key, value := range response {
		normalized[key] = value
	}
	if traceID, hasTraceID := root["traceId"].(string); hasTraceID {
		normalized["responseId"] = traceID
	}
	return normalized
}
