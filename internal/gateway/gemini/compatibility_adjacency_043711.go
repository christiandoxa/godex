package gemini

import (
	"errors"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

// The tagged 0.438.4 adjacency kernel keeps selected calls and prose in one
// assistant message, followed by first outputs in call order.
func filterGeminiOrphanToolMessages043711(body []byte) ([]byte, error) {
	request, err := decodeGeminiChatRequest04384(body)
	if err != nil {
		return nil, errors.New("invalid Gemini chat-compatible request")
	}
	messages, _ := request["messages"].([]any)
	firstOutput := make(map[string]int)
	for i, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] != "tool" {
			continue
		}
		id, _ := message["tool_call_id"].(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		if _, exists := firstOutput[id]; !exists {
			firstOutput[id] = i
		}
	}
	matchedID := make(map[string]bool)
	claimedOutput := make(map[int]bool)
	result := make([]any, 0, len(messages))
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message == nil {
			result = append(result, raw)
			continue
		}
		// With indexed outputs, only matched call/output pairs are re-emitted.
		if message["role"] == "tool" && len(firstOutput) > 0 {
			continue
		}
		calls, hasCalls := message["tool_calls"].([]any)
		if !hasCalls || len(calls) == 0 {
			result = append(result, raw)
			continue
		}
		selected := make([]any, 0, len(calls))
		for _, candidate := range calls {
			call, _ := candidate.(map[string]any)
			id, _ := call["id"].(string)
			index, present := firstOutput[id]
			if !present || strings.TrimSpace(id) == "" || matchedID[id] ||
				claimedOutput[index] {
				continue
			}
			matchedID[id] = true
			claimedOutput[index] = true
			selected = append(selected, candidate)
		}
		if len(selected) > 0 {
			caller := copyGeminiMessage043711(message)
			caller["tool_calls"] = selected
			if caller["role"] == "assistant" && caller["content"] == nil {
				caller["content"] = ""
			}
			result = append(result, caller)
			for _, candidate := range selected {
				call := candidate.(map[string]any)
				id := call["id"].(string)
				result = append(result, messages[firstOutput[id]])
			}
			continue
		}
		prose := copyGeminiMessage043711(message)
		delete(prose, "tool_calls")
		if geminiMessageHasIndependentContent043711(prose) {
			result = append(result, prose)
		}
	}
	if len(result) == 0 {
		result = append(result, map[string]any{"role": "user", "content": ""})
	}
	request["messages"] = result
	return jsonutil.MarshalSerdeValue04384(request)
}

func copyGeminiMessage043711(value map[string]any) map[string]any {
	copyValue := make(map[string]any, len(value))
	for key, field := range value {
		copyValue[key] = field
	}
	return copyValue
}

func geminiMessageHasIndependentContent043711(value map[string]any) bool {
	if text, ok := value["content"].(string); ok && strings.TrimSpace(text) != "" {
		return true
	}
	if content, exists := value["content"]; exists && content != nil {
		if _, ok := content.(string); !ok {
			return true
		}
	}
	// The tagged adjacency kernel retains assistant reasoning and
	// provider-specific metadata even without user-facing prose.
	for _, key := range []string{"reasoning_content", "refusal", "annotations"} {
		if field, exists := value[key]; exists && field != nil {
			return true
		}
	}
	return false
}
