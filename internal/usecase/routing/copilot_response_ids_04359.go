package routing

import (
	"encoding/json"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

// copilotResponseIDs mirrors Prodex 0.435.9's Copilot binding recorder.
// Unlike generic Responses metadata, provider-native JSON may use a root id,
// camel responseId, or message.id without object:"response". The caller
// applies this policy only when the selected upstream provider is Copilot.
func copilotResponseIDs(body []byte, stream bool) []string {
	if len(body) == 0 {
		return nil
	}
	var items [][]byte
	if stream {
		decoder := sse.NewDecoder(len(body))
		items = decoder.Feed(body)
	} else {
		items = [][]byte{body}
	}
	var ids []string
	seen := make(map[string]bool)
	for _, data := range items {
		var object map[string]any
		if json.Unmarshal(data, &object) != nil {
			continue
		}
		id := copilotResponseIDFromValue(object)
		if id == "" || seen[id] {
			continue
		}
		ids = append(ids, id)
		seen[id] = true
	}
	return ids
}

func copilotResponseIDFromValue(object map[string]any) string {
	if object == nil {
		return ""
	}
	// The Mojo provider-core parser tests raw string *presence* before
	// trimming. A whitespace-only nested ID suppresses the root ID fallback.
	var value string
	var present bool
	if response, ok := object["response"].(map[string]any); ok {
		value, present = response["id"].(string)
	}
	if !present {
		value, present = object["id"].(string)
	}
	if !present {
		value, present = object["response_id"].(string)
	}
	if id := strings.TrimSpace(value); id != "" {
		return id
	}

	// Prodex's local rewrite bridge accepts native provider response shapes
	// as fallback only if the canonical provider-core parser found no ID.
	if candidate, ok := object["responseId"].(string); ok {
		// The Rust wrapper selects the first present native string, then
		// trims once at the end. Blank responseId suppresses message.id.
		return strings.TrimSpace(candidate)
	}
	if message, ok := object["message"].(map[string]any); ok {
		if candidate, ok := message["id"].(string); ok {
			return strings.TrimSpace(candidate)
		}
	}
	return ""
}
