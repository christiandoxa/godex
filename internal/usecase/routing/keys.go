package routing

import (
	"encoding/json"
	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"net/http"
	"strings"
)

func requestAffinity(request proxymodel.Request, body []byte) affinityKeys {
	keys := affinityKeys{
		thread:  firstHeader(request.Header, "thread-id", "x-codex-thread-id"),
		turn:    strings.TrimSpace(request.Header.Get("x-codex-turn-state")),
		session: firstHeader(request.Header, "x-codex-session-id", "x-session-id", "session-id"),
	}
	if len(body) == 0 {
		return keys
	}
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return keys
	}
	keys.previous = objectString(object, "previous_response_id")
	if keys.previous == "" {
		if response, ok := nestedAffinityObject("response", object["response"]); ok {
			keys.previous = objectString(response, "previous_response_id")
		}
	}
	if keys.session == "" {
		keys.session = objectString(object, "session_id", "conversation_id", "thread_id")
		if keys.session == "" {
			if metadata, ok := object["client_metadata"].(map[string]any); ok {
				keys.session = objectString(metadata, "session_id")
			}
		}
	}
	return keys
}

func responseAffinity(headers http.Header, body []byte, stream bool) affinityKeys {
	keys := affinityKeys{
		thread:  firstHeader(headers, "thread-id", "x-codex-thread-id"),
		turn:    strings.TrimSpace(headers.Get("x-codex-turn-state")),
		session: firstHeader(headers, "x-codex-session-id", "x-session-id", "session-id"),
	}
	if len(body) == 0 {
		return keys
	}
	if stream {
		decoder := sse.NewDecoder(len(body))
		for _, data := range decoder.Feed(body) {
			var object map[string]any
			if json.Unmarshal(data, &object) == nil {
				keys = mergeAffinityKeys(keys, responseObjectAffinity(object))
			}
		}
		return keys
	}
	var object map[string]any
	if json.Unmarshal(body, &object) == nil {
		return mergeAffinityKeys(keys, responseObjectAffinity(object))
	}
	return keys
}

func responseObjectAffinity(object map[string]any) affinityKeys {
	keys := affinityKeys{
		previous: objectString(object, "id", "response_id"),
		session:  objectString(object, "session_id", "conversation_id", "thread_id"),
		turn:     objectString(object, "turn_state"),
	}
	collectNestedAffinity(object, &keys, 0)
	return keys
}

func collectNestedAffinity(object map[string]any, keys *affinityKeys, depth int) {
	if depth >= 3 {
		return
	}
	for name, value := range object {
		nested, ok := nestedAffinityObject(name, value)
		if !ok {
			continue
		}
		mergeNestedAffinity(keys, nested)
		collectNestedAffinity(nested, keys, depth+1)
	}
}

func nestedAffinityObject(name string, value any) (map[string]any, bool) {
	switch name {
	case "response", "data", "result", "payload":
		nested, ok := value.(map[string]any)
		return nested, ok
	default:
		return nil, false
	}
}

func mergeNestedAffinity(keys *affinityKeys, nested map[string]any) {
	if keys.previous == "" {
		keys.previous = objectString(nested, "id", "response_id")
	}
	if keys.session == "" {
		keys.session = objectString(nested, "session_id", "conversation_id", "thread_id")
	}
	if keys.turn == "" {
		keys.turn = objectString(nested, "turn_state")
	}
}

func mergeAffinityKeys(first, second affinityKeys) affinityKeys {
	if first.previous == "" {
		first.previous = second.previous
	}
	if first.turn == "" {
		first.turn = second.turn
	}
	if first.session == "" {
		first.session = second.session
	}
	return first
}

func firstHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func objectString(object map[string]any, names ...string) string {
	for _, name := range names {
		if value, ok := object[name].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
