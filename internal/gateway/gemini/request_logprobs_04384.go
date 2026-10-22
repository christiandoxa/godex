package gemini

import (
	"encoding/json"
	"strconv"
)

// Keep Gemini controls outside the shared Claude converter's rejected fields.
func extractGeminiChatLogprobs04384(request map[string]any) (map[string]any, bool) {
	enabled, present := request["logprobs"]
	if present {
		if _, valid := enabled.(bool); !valid {
			return nil, false
		}
	}
	if top, present := request["top_logprobs"]; present {
		number, valid := top.(json.Number)
		count, err := strconv.ParseUint(string(number), 10, 64)
		if !valid || err != nil || count > 20 || enabled != true {
			return nil, false
		}
	}
	var controls map[string]any
	for _, key := range []string{"logprobs", "top_logprobs"} {
		if value, present := request[key]; present {
			if controls == nil {
				controls = make(map[string]any, 2)
			}
			controls[key] = value
			delete(request, key)
		}
	}
	return controls, true
}
