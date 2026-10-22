package gemini

func extractGeminiChatStop04384(request, controls map[string]any) (map[string]any, bool) {
	stop, present := firstPresent(request, "stop", "stop_sequences", "stopSequences")
	if !present {
		return controls, true
	}
	switch value := stop.(type) {
	case string:
	case []any:
		if len(value) > 16 {
			return nil, false
		}
		for _, entry := range value {
			if _, valid := entry.(string); !valid {
				return nil, false
			}
		}
	default:
		return nil, false
	}
	if controls == nil {
		controls = make(map[string]any, 1)
	}
	controls["stop"] = stop
	for _, key := range []string{"stop", "stop_sequences", "stopSequences"} {
		delete(request, key)
	}
	return controls, true
}
