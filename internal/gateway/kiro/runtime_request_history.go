package kiro

func firstKiroToolOutputCallID(value any) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || runtimeString(item["type"], "") != "function_call_output" {
			continue
		}
		if callID := runtimeString(item["call_id"], ""); callID != "" {
			return callID
		}
	}
	return ""
}

func kiroFunctionCallIDs(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || runtimeString(item["type"], "") != "function_call" {
			continue
		}
		if callID := runtimeString(item["call_id"], ""); callID != "" {
			result = append(result, callID)
		}
	}
	return result
}
