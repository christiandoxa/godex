package gemini

import (
	"errors"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

// The tagged 0.437.11 Gemini API-key chat-compatible bridge invokes the
// Prodex/DeepSeek message-adjacency kernel. Calls with an available output
// are emitted immediately before that output, even if the output was
// received out of order. Unanswered calls are removed; assistant prose
// is preserved separately after the tool sequence.
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
	// Output may precede its call in a repaired history. Reserve every
	// first matching output before emitting anything, so the original
	// location (and duplicates) are not replayed a second time.
	eligible := make(map[string]bool)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		calls, _ := message["tool_calls"].([]any)
		for _, candidate := range calls {
			call, _ := candidate.(map[string]any)
			id, _ := call["id"].(string)
			if _, found := firstOutput[id]; found && strings.TrimSpace(id) != "" {
				eligible[id] = true
			}
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
		calls, hasCalls := message["tool_calls"].([]any)
		if !hasCalls || len(calls) == 0 {
			// Skip matched tool results here: they have already been
			// emitted next to their corresponding call.
			if message["role"] == "tool" {
				id, _ := message["tool_call_id"].(string)
				if eligible[id] && strings.TrimSpace(id) != "" {
					continue
				}
			}
			result = append(result, raw)
			continue
		}
		paired := 0
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
			paired++
			caller := copyGeminiMessage043711(message)
			caller["tool_calls"] = []any{candidate}
			if caller["role"] == "assistant" {
				caller["content"] = ""
			}
			result = append(result, caller, messages[index])
		}
		prose := copyGeminiMessage043711(message)
		delete(prose, "tool_calls")
		if geminiMessageHasIndependentContent043711(prose) {
			result = append(result, prose)
		} else if paired == 0 {
			// A message with only unanswered tool calls and no visible
			// prose is not a usable continuation. Other messages are
			// retained by geminiMessageHasIndependentContent043711.
		}
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
