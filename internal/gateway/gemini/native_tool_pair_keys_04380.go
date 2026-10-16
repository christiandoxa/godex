package gemini

// The compiled Gemini content-hardening pairing ABI treats the *presence*
// of functionCall/functionResponse as a tool part, including JSON null. A
// malformed/missing id defaults to "", and name defaults to "unknown".
func geminiToolIdentity04380(value any, key string) string {
	fallback := "unknown"
	if key == "id" {
		fallback = ""
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fallback
	}
	result, ok := object[key].(string)
	if !ok {
		return fallback
	}
	return result
}

func geminiToolValues04380(content any, kind string) []any {
	object, ok := content.(map[string]any)
	if !ok {
		return nil
	}
	parts, ok := object["parts"].([]any)
	if !ok {
		return nil
	}
	values := make([]any, 0)
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if value, present := part[kind]; present {
			values = append(values, value)
		}
	}
	return values
}

func geminiMatchingTool04380(value any, other any, kind string) bool {
	for _, candidate := range geminiToolValues04380(other, kind) {
		if geminiToolIdentity04380(value, "id") == geminiToolIdentity04380(candidate, "id") &&
			geminiToolIdentity04380(value, "name") == geminiToolIdentity04380(candidate, "name") {
			return true
		}
	}
	return false
}

func geminiMissingParts04380(source, other any, orphan bool) []any {
	sourceKind, counterpart := "functionCall", "functionResponse"
	if orphan {
		sourceKind, counterpart = "functionResponse", "functionCall"
	}
	missing := make([]any, 0)
	for _, value := range geminiToolValues04380(source, sourceKind) {
		if geminiMatchingTool04380(value, other, counterpart) {
			continue
		}
		id, name := geminiToolIdentity04380(value, "id"), geminiToolIdentity04380(value, "name")
		if orphan {
			missing = append(missing, map[string]any{
				"functionCall":     map[string]any{"id": id, "name": name, "args": map[string]any{}},
				"thoughtSignature": "skip_thought_signature_validator",
			})
		} else {
			missing = append(missing, map[string]any{
				"functionResponse": map[string]any{"id": id, "name": name, "response": map[string]any{
					"error": "The tool execution result was missing from the Codex conversation history. Treat this tool result as unavailable and recover by inspecting current workspace state before continuing.",
				}},
			})
		}
	}
	return missing
}

func geminiToolRole04380(row any, role string) bool {
	object, ok := row.(map[string]any)
	return ok && object["role"] == role
}

func geminiExtendedToolContent04380(content any, missing []any) any {
	object := content.(map[string]any)
	copied := make(map[string]any, len(object))
	for key, value := range object {
		copied[key] = value
	}
	parts := append([]any(nil), object["parts"].([]any)...)
	copied["parts"] = append(parts, missing...)
	return copied
}
