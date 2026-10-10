package gemini

func geminiNativeTools(chat map[string]any) []any {
	items, _ := chat["tools"].([]any)
	declarations := make([]any, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		function, _ := item["function"].(map[string]any)
		if function == nil {
			continue
		}
		declaration := make(map[string]any)
		for _, key := range []string{"name", "description", "parameters"} {
			if value, ok := function[key]; ok {
				declaration[key] = value
			}
		}
		if _, ok := declaration["name"]; ok {
			declarations = append(declarations, declaration)
		}
	}
	if len(declarations) == 0 {
		return nil
	}
	return []any{map[string]any{"functionDeclarations": declarations}}
}

func geminiNativeToolConfig(value any) map[string]any {
	if value == nil {
		return nil
	}
	config := map[string]any{}
	switch value := value.(type) {
	case string:
		switch value {
		case "none":
			config["mode"] = "NONE"
		case "required":
			config["mode"] = "ANY"
		case "auto":
			return nil
		}
	case map[string]any:
		function, _ := value["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name == "" {
			name, _ = value["name"].(string)
		}
		if name != "" {
			config["mode"] = "ANY"
			config["allowedFunctionNames"] = []any{name}
		}
	}
	if len(config) == 0 {
		return nil
	}
	return map[string]any{"functionCallingConfig": config}
}
