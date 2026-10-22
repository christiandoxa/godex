package gemini

import (
	"encoding/json"
	"strconv"
)

func normalizeGeminiTokenLimit04384(request map[string]any) bool {
	var selected json.Number
	for _, field := range []string{"max_output_tokens", "max_tokens", "max_completion_tokens"} {
		value, present := request[field]
		if !present {
			continue
		}
		number, valid := value.(json.Number)
		count, err := strconv.ParseUint(string(number), 10, 64)
		if !valid || err != nil || count == 0 {
			return false
		}
		selected = number
	}
	if selected != "" {
		delete(request, "max_output_tokens")
		delete(request, "max_completion_tokens")
		request["max_tokens"] = selected
	}
	return true
}
