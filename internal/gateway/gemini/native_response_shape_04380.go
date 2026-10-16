package gemini

import "strings"

func firstNativeCandidate(root map[string]any) map[string]any {
	candidates, _ := root["candidates"].([]any)
	if len(candidates) == 0 {
		return nil
	}
	candidate, _ := candidates[0].(map[string]any)
	return candidate
}

func nativeCandidateParts(candidate map[string]any) []any {
	if candidate == nil {
		return nil
	}
	content, _ := candidate["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	return parts
}

func nativeString(value any) string {
	text, _ := value.(string)
	return text
}

func nativeNonblank(value any) string {
	text := nativeString(value)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return text
}

func firstPresentNative(object map[string]any, keys ...string) any {
	for _, key := range keys {
		if object != nil {
			if value, ok := object[key]; ok {
				return value
			}
		}
	}
	return nil
}
