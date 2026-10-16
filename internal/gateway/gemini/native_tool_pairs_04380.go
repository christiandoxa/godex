package gemini

// Tagged Prodex 0.438.0 Mojo Gemini content-hardening operation 1 appends
// unavailable tool results by matching BOTH ID and name, retaining repeated
// missing calls. It never executes tools and never rewrites existing results.
func geminiRepairMissingToolResults04380(contents []any) []any {
	result := make([]any, 0, len(contents)+2)
	for index, current := range contents {
		var previous, next any
		if index > 0 {
			previous = contents[index-1]
		}
		if index+1 < len(contents) {
			next = contents[index+1]
		}
		if geminiToolRole04380(current, "user") && geminiToolRole04380(previous, "model") {
			if _, valid := geminiContentParts04380(current); valid {
				missing := geminiMissingParts04380(previous, current, false)
				if len(missing) > 0 {
					current = geminiExtendedToolContent04380(current, missing)
				}
			}
		}
		result = append(result, current)
		if !geminiToolRole04380(current, "model") {
			continue
		}
		_, nextPartsValid := geminiContentParts04380(next)
		if geminiToolRole04380(next, "user") && nextPartsValid {
			continue
		}
		missing := geminiMissingParts04380(current, nil, false)
		if len(missing) > 0 {
			result = append(result, map[string]any{"role": "user", "parts": missing})
		}
	}
	return result
}

// Tagged Mojo operation 2 synthesizes unavailable call metadata only for
// genuinely orphaned tool responses. A malformed previous model `parts`
// remains untouched exactly as the official source tests require.
func geminiRepairOrphanToolResponses04380(contents []any) []any {
	result := make([]any, 0, len(contents)+2)
	for index, current := range contents {
		var previous, next any
		if index > 0 {
			previous = contents[index-1]
		}
		if index+1 < len(contents) {
			next = contents[index+1]
		}
		if geminiToolRole04380(current, "user") && !geminiToolRole04380(previous, "model") {
			missing := geminiMissingParts04380(current, nil, true)
			if len(missing) > 0 {
				result = append(result, map[string]any{"role": "model", "parts": missing})
			}
		}
		if geminiToolRole04380(current, "model") && geminiToolRole04380(next, "user") {
			if _, valid := geminiContentParts04380(current); valid {
				missing := geminiMissingParts04380(next, current, true)
				if len(missing) > 0 {
					current = geminiExtendedToolContent04380(current, missing)
				}
			}
		}
		result = append(result, current)
	}
	return result
}

func geminiContentParts04380(content any) ([]any, bool) {
	object, ok := content.(map[string]any)
	if !ok {
		return nil, false
	}
	parts, ok := object["parts"].([]any)
	return parts, ok
}
