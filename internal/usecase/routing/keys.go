package routing

import (
	"encoding/json"
	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"net/http"
	"strings"
	"unicode/utf8"
)

func requestAffinity(request proxymodel.Request, body []byte) affinityKeys {
	keys := affinityKeys{
		thread:  firstHeader(request.Header, "thread-id", "x-codex-thread-id"),
		turn:    strings.TrimSpace(request.Header.Get("x-codex-turn-state")),
		session: firstHeader(request.Header, "session_id", "session-id", "x-session-id"),
	}
	if keys.session == "" {
		var metadata map[string]any
		if json.Unmarshal([]byte(request.Header.Get("x-codex-turn-metadata")), &metadata) == nil {
			keys.session = objectString(metadata, "session_id")
		}
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
		session: firstHeader(headers, "session_id", "session-id", "x-session-id"),
	}
	if len(body) == 0 {
		return keys
	}
	if stream {
		decoder := sse.NewDecoder(len(body))
		events := decoder.Feed(body)
		events = append(events, decoder.Finish()...)
		for _, data := range events {
			if !utf8.Valid(data) {
				continue
			}
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
		previous: responseID(object),
		session:  objectString(object, "session_id", "conversation_id", "thread_id"),
		turn:     responseObjectTurnState(object),
	}
	collectNestedAffinity(object, &keys, 0)
	return keys
}

func responseID(object map[string]any) string {
	if response, ok := object["response"].(map[string]any); ok {
		if id := objectString(response, "id"); id != "" {
			return id
		}
	}
	if id := objectString(object, "response_id"); id != "" {
		return id
	}
	objectKind := objectString(object, "object")
	if objectKind == "response" || strings.HasSuffix(objectKind, ".response") {
		return objectString(object, "id")
	}
	return ""
}

func responseObjectTurnState(object map[string]any) string {
	response, _ := object["response"].(map[string]any)
	if response != nil {
		if state := responseHeaderTurnState(response["headers"]); state != "" {
			return state
		}
	}
	if state := responseHeaderTurnState(object["headers"]); state != "" {
		return state
	}
	if response != nil {
		if state := objectString(response, "turn_state", "turnState"); state != "" {
			return state
		}
	}
	return objectString(object, "turn_state", "turnState")
}

func responseHeaderTurnState(headers any) string {
	switch value := headers.(type) {
	case map[string]any:
		for name, raw := range value {
			if strings.EqualFold(strings.TrimSpace(name), "x-codex-turn-state") {
				if state := responseHeaderValue(raw); state != "" {
					return state
				}
			}
		}
	case []any:
		for _, raw := range value {
			var name, value any
			switch entry := raw.(type) {
			case []any:
				if len(entry) >= 2 {
					name, value = entry[0], entry[1]
				}
			case map[string]any:
				name = entry["name"]
				if name == nil {
					name = entry["key"]
				}
				value = entry["value"]
				if value == nil {
					value = entry["values"]
				}
			}
			nameText, _ := name.(string)
			if strings.EqualFold(strings.TrimSpace(nameText), "x-codex-turn-state") {
				if state := responseHeaderValue(value); state != "" {
					return state
				}
			}
		}
	}
	return ""
}

func responseHeaderValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		for _, item := range typed {
			if text := responseHeaderValue(item); text != "" {
				return text
			}
		}
	}
	return ""
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
