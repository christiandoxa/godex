package routing

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"net/http"
	"sort"
	"strings"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func requestAffinity(request proxymodel.Request, body []byte) affinityKeys {
	thread := firstHeader(request.Header, "thread-id")
	// Prodex's HTTP Responses and WebSocket runtimes only use thread-id
	// for trace attribution. It does not create hard continuation routing
	// affinity on these lanes. Keep the legacy standard proxy behavior.
	path := strings.TrimRight(request.Path, "/")
	if request.WebSocketMessage || request.QuotaSelection.RouteKind != quotamodel.RouteKindStandard ||
		strings.HasSuffix(path, "/responses") || strings.HasSuffix(path, "/responses/compact") {
		thread = ""
	}
	keys := affinityKeys{
		thread:  thread,
		turn:    strings.TrimSpace(request.Header.Get("x-codex-turn-state")),
		session: firstHeader(request.Header, "session_id", "session-id", "x-session-id"),
	}
	if keys.session == "" {
		value, valid := decodeNativeUsageJSON([]byte(request.Header.Get("x-codex-turn-metadata")))
		if metadata, ok := value.(map[string]any); valid && ok {
			// Turn metadata, too, is semantically meaningful only when the
			// entire serde JSON document is valid and complete.
			keys.session = semanticNestedText(metadata, "session_id", "client_metadata", "session_id")
		}
	}
	if len(body) == 0 {
		return keys
	}
	value, valid := decodeNativeUsageJSON(body)
	object, ok := value.(map[string]any)
	if !valid || !ok {
		return keys
	}
	if keys.turn == "" && request.WebSocketMessage {
		// Tagged WebSocket request semantics accept only this exact member
		// or its client_metadata counterpart. The response-only turn_state
		// field cannot pin a request to another continuation owner.
		keys.turn = semanticNestedText(object, "x-codex-turn-state", "client_metadata", "x-codex-turn-state")
	}
	// Tagged Prodex's HTTP and WebSocket request semantic planner reads
	// previous_response_id only from the direct JSON member. A nested
	// response object is opaque request payload, never hard owner evidence.
	keys.previous = objectString(object, "previous_response_id")
	if keys.session == "" {
		_, direct := object["session_id"]
		// Tagged Prodex's semantic planner uses direct JSON member presence
		// on both HTTP Responses and WebSocket requests, not merely whether
		// its text is nonempty. A null/blank direct member must not select a
		// foreign session from nested client_metadata.
		if direct {
			keys.session = nonblankJSONText(object["session_id"])
		} else if metadata, ok := object["client_metadata"].(map[string]any); ok {
			// The canonical request session key has no conversation_id or
			// thread_id aliases. Those fields are opaque request payload.
			keys.session = objectString(metadata, "session_id")
		}
	}
	return keys
}

// Hard continuation keys outrank a soft session preference on Responses and
// WebSocket routes. Keep the original keys for cleanup and successful rebinding.
func requestAffinityForRoute(request proxymodel.Request, body []byte, route quotamodel.RouteKind) affinityKeys {
	request.QuotaSelection.RouteKind = route
	keys := requestAffinity(request, body)
	return affinityKeysForRoute(keys, route)
}

func affinityKeysForRoute(keys affinityKeys, route quotamodel.RouteKind) affinityKeys {
	if (route == quotamodel.RouteKindResponses || route == quotamodel.RouteKindWebSocket) &&
		(keys.previous != "" || keys.turn != "" || keys.thread != "") {
		keys.session = ""
	}
	if route == quotamodel.RouteKindCompact && keys.previous != "" {
		keys.session = ""
	}
	return keys
}

func responseAffinity(headers http.Header, body []byte, stream bool) affinityKeys {
	keys, _ := responseAffinityWithIDs(headers, body, stream)
	return keys
}

func responseObjectAffinity(object map[string]any) affinityKeys {
	// The tagged response metadata planner accepts only the direct
	// response.id, top-level response_id / qualifying id, and response or
	// root turn-state. Arbitrary data/result/payload wrappers are opaque:
	// inspecting them as hard continuation metadata can steal an owner.
	return affinityKeys{
		previous: responseID(object),
		turn:     responseObjectTurnState(object),
	}
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
	if responseKindAdmitsRootID04383(object) {
		return objectString(object, "id")
	}
	return ""
}

// The compiled response metadata planner matches the serde_json-decoded
// `object` text exactly: "response" or an untrimmed ".response" suffix.
// Unlike an identifier value, whitespace here changes the event kind and
// must never turn an opaque root id into a hard continuation owner.
func responseKindAdmitsRootID04383(object map[string]any) bool {
	kind, _ := object["object"].(string)
	return kind == "response" || strings.HasSuffix(kind, ".response")
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
		// serde_json object maps are sorted by raw key bytes before Mojo's
		// first-matching-entry metadata scan; Go map iteration is randomized.
		names := make([]string, 0, len(value))
		for name := range value {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if responseTurnHeaderName04383(name) {
				if state := responseHeaderValue(value[name]); state != "" {
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
				// In the tagged Mojo metadata planner, a present JSON null
				// shadows the fallback field; only ABSENT name/value members
				// may borrow key/values from this header entry.
				var found bool
				name, found = entry["name"]
				if !found {
					name = entry["key"]
				}
				value, found = entry["value"]
				if !found {
					value = entry["values"]
				}
			}
			nameText, _ := name.(string)
			if responseTurnHeaderName04383(nameText) {
				if state := responseHeaderValue(value); state != "" {
					return state
				}
			}
		}
	}
	return ""
}

// The compiled 0.438.3 JSON metadata scanner accepts only exact-length
// ASCII-casefolded header names, not Unicode lookalikes or padded names.
func responseTurnHeaderName04383(name string) bool {
	const header = "x-codex-turn-state"
	return len(name) == len(header) && strings.EqualFold(name, header)
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
