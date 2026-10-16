package main

import "strings"

// Source: Prodex 0.438.0 at 8cab9794b0e08b2cb8507df8321a8e38ebd6492f;
// these bodies mirror its DeepSeek input-shape tests.
func isKnownDeepSeekInvalidInput(name string) bool {
	for _, known := range deepSeekInvalidInputScenarioNames() {
		if name == known {
			return true
		}
	}
	return false
}

func deepSeekInvalidInputScenarioNames() []string {
	return []string{
		"deepseek-unsupported-message-role",
		"deepseek-null-message-content",
		"deepseek-request-over-kernel-limit",
		"deepseek-input-type-null",
		"deepseek-input-type-number",
		"deepseek-input-type-empty",
		"deepseek-empty-top-level-tool-name",
		"deepseek-invalid-nested-shell-command",
	}
}

func deepSeekInvalidInputFixture(name string) []byte {
	switch name {
	case "deepseek-unsupported-message-role":
		return []byte(`{"model":"deepseek-v4-pro","input":[{"type":"message","role":"critic","content":"no"}],"stream":false}`)
	case "deepseek-null-message-content":
		return []byte(`{"model":"deepseek-v4-pro","input":[{"type":"message","role":"user","content":null}],"stream":false}`)
	case "deepseek-request-over-kernel-limit":
		return []byte(`{"model":"deepseek-v4-pro","input":"` + strings.Repeat("x", 4<<20) + `","stream":false}`)
	case "deepseek-input-type-null":
		return []byte(`{"model":"deepseek-v4-pro","input":[{"type":null,"content":"note"}],"stream":false}`)
	case "deepseek-input-type-number":
		return []byte(`{"model":"deepseek-v4-pro","input":[{"type":1,"content":"note"}],"stream":false}`)
	case "deepseek-input-type-empty":
		return []byte(`{"model":"deepseek-v4-pro","input":[{"type":"","content":"note"}],"stream":false}`)
	case "deepseek-empty-top-level-tool-name":
		return []byte(`{"model":"deepseek-v4-pro","input":[{"type":"function_call","call_id":"call_1","name":"","function":{"name":"lookup"}}],"stream":false}`)
	case "deepseek-invalid-nested-shell-command":
		return []byte(`{"model":"deepseek-v4-pro","input":[{"type":"local_shell_call","call_id":"call_1","action":{"command":[]},"command":"fallback"}],"stream":false}`)
	default:
		return nil
	}
}
