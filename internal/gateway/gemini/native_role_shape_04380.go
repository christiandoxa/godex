package gemini

// Tagged Prodex 0.438.0 gch_coalesce (operation 0) groups adjacent content
// entries with identical role and keeps their part order. The first row's
// metadata owns the combined row; later metadata must not replace it.
func geminiCoalesceContents04380(contents []any) []any {
	grouped := make([]any, 0, len(contents))
	for _, raw := range contents {
		content, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok || len(parts) == 0 {
			continue
		}
		role, ok := content["role"].(string)
		if !ok {
			role = "user"
		}
		if len(grouped) > 0 {
			previous := grouped[len(grouped)-1].(map[string]any)
			if previous["role"] == role {
				previous["parts"] = append(previous["parts"].([]any), parts...)
				continue
			}
		}
		copyValue := make(map[string]any, len(content)+1)
		for key, value := range content {
			copyValue[key] = value
		}
		copyValue["role"] = role
		copyValue["parts"] = append([]any(nil), parts...)
		grouped = append(grouped, copyValue)
	}
	return grouped
}

// Operation 3 requires user-shaped boundaries for a plain model-led or
// model-ended transcript. The separate Mojo operations 1/2 own missing tool
// result pairing: do NOT manufacture a text prompt for a tool-only request.
func geminiShapePlainContents04380(contents []any) []any {
	// The tagged operation 0 coalesces all roles, but the full production
	// pipeline pairs and orders tool calls/results between coalescing and
	// role shaping. The Go native adapter has its own existing tool-order
	// contract; do not partially apply operation 0 to tool-bearing turns
	// without also reproducing the separate pairing operations 1, 2 and 4.
	if geminiContentsContainToolParts04380(contents) {
		return contents
	}
	grouped := geminiCoalesceContents04380(contents)
	if len(grouped) == 0 {
		return grouped
	}
	makeUser := func(text string) any {
		return map[string]any{"role": "user", "parts": []any{map[string]any{"text": text}}}
	}
	if grouped[0].(map[string]any)["role"] == "model" {
		grouped = append([]any{makeUser("[Continuing from previous Codex context.]")}, grouped...)
	}
	if grouped[len(grouped)-1].(map[string]any)["role"] == "model" {
		grouped = append(grouped, makeUser("Please continue."))
	}
	return grouped
}

func geminiContentsContainToolParts04380(contents []any) bool {
	for _, raw := range contents {
		content := raw.(map[string]any)
		for _, part := range content["parts"].([]any) {
			object, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := object["functionCall"]; ok {
				return true
			}
			if _, ok := object["functionResponse"]; ok {
				return true
			}
		}
	}
	return false
}
