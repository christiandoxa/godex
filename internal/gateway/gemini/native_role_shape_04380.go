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
	// pipeline pairs and orders tools between coalescing and role shaping.
	// Until operations 1/2/4 are parity-tested, preserve tool-bearing turns.
	if geminiContentsContainToolParts04380(contents) {
		// The complete tagged pipeline coalesces adjacent same-role tool
		// rows BEFORE applying operations 1/2 and operation 4 ordering.
		// Preserve compound histories until that pipeline is verified.
		// Single model/user pairs, isolated missing calls and orphan results
		// can use the tagged bounded pairing policy without reordering.
		for index := 1; index < len(contents); index++ {
			previous, okPrevious := contents[index-1].(map[string]any)
			current, okCurrent := contents[index].(map[string]any)
			if okPrevious && okCurrent && previous["role"] == current["role"] {
				return contents
			}
		}
		return geminiRepairOrphanToolResponses04380(
			geminiRepairMissingToolResults04380(contents),
		)
	}

	// Prodex's operation 3 makes lead/tail decisions from the RAW first and
	// last rows, *before* discarding invalid rows with no parts. A null row
	// at the front prevents a synthetic lead; an invalid trailing model row
	// still requires a synthetic final user turn.
	lead := len(contents) > 0 && geminiRawModelRole04380(contents[0])
	tail := len(contents) > 0 && geminiRawModelRole04380(contents[len(contents)-1])
	grouped := geminiCoalesceContents04380(contents)
	makeUser := func(text string) map[string]any {
		return map[string]any{"role": "user", "parts": []any{map[string]any{"text": text}}}
	}
	if len(grouped) == 0 {
		if !lead && !tail {
			return grouped
		}
		user := map[string]any{"role": "user", "parts": []any{}}
		if lead {
			user["parts"] = append(user["parts"].([]any), map[string]any{"text": "[Continuing from previous Codex context.]"})
		}
		if tail {
			user["parts"] = append(user["parts"].([]any), map[string]any{"text": "Please continue."})
		}
		return []any{user}
	}
	if lead {
		first := grouped[0].(map[string]any)
		if first["role"] == "user" {
			// gch_write_group(lead=true) replaces the retained metadata with
			// synthetic-leading metadata rather than copying the first row.
			grouped[0] = map[string]any{"role": "user", "parts": append(
				[]any{map[string]any{"text": "[Continuing from previous Codex context.]"}},
				first["parts"].([]any)...,
			)}
		} else {
			grouped = append([]any{makeUser("[Continuing from previous Codex context.]")}, grouped...)
		}
	}
	if tail {
		last := grouped[len(grouped)-1].(map[string]any)
		if last["role"] == "user" {
			// Last raw model entry was filtered: the synthetic trailing
			// prompt merges into the already retained user group.
			last["parts"] = append(last["parts"].([]any), map[string]any{"text": "Please continue."})
		} else {
			grouped = append(grouped, makeUser("Please continue."))
		}
	}
	return grouped
}

func geminiRawModelRole04380(raw any) bool {
	object, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	role, ok := object["role"].(string)
	return ok && role == "model"
}

func geminiContentsContainToolParts04380(contents []any) bool {
	for _, raw := range contents {
		content, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		for _, part := range parts {
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
