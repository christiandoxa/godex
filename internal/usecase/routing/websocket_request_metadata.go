package routing

import (
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketRequestMetadata struct {
	previousResponseID               string
	sessionID                        string
	turnState                        string
	requiresPreviousResponseAffinity bool
}

func parseWebSocketRequestMetadata(request proxymodel.Request) websocketRequestMetadata {
	var metadata websocketRequestMetadata
	decoded, valid := decodeNativeUsageJSON(request.Body)
	if value, ok := decoded.(map[string]any); valid && ok {
		// Tagged parse_runtime_websocket_request_metadata starts from a
		// full serde_json::Value. Go's json.Unmarshal would accept a near-
		// maximum f64 that the source rejects, falsely pinning tool-output
		// to another verified previous-response owner.
		metadata.previousResponseID = nonblankJSONText(value["previous_response_id"])
		metadata.sessionID = semanticNestedText(value, "session_id", "client_metadata", "session_id")
		metadata.turnState = semanticNestedText(value, "x-codex-turn-state", "client_metadata", "x-codex-turn-state")
		metadata.requiresPreviousResponseAffinity =
			metadata.previousResponseID != "" && requestInputHasToolOutput(value["input"])
	}
	if session := explicitWebSocketSessionID(request); session != "" {
		metadata.sessionID = session
	}
	if metadata.turnState == "" {
		metadata.turnState = strings.TrimSpace(request.Header.Get("x-codex-turn-state"))
	}
	return metadata
}

func explicitWebSocketSessionID(request proxymodel.Request) string {
	for _, name := range []string{"session_id", "session-id", "x-session-id"} {
		if value := strings.TrimSpace(request.Header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func semanticNestedText(
	value map[string]any,
	directName, nestedObjectName, nestedName string,
) string {
	// Tagged Prodex's runtime_request_semantic_plan selects a direct JSON
	// member by presence, not by whether its value is a usable string.
	// A present empty/invalid member must not resurrect a nested affinity.
	if direct, present := value[directName]; present {
		return nonblankJSONText(direct)
	}
	nested, ok := value[nestedObjectName].(map[string]any)
	if !ok {
		return ""
	}
	return nonblankJSONText(nested[nestedName])
}

func nonblankJSONText(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func requestInputHasToolOutput(value any) bool {
	input, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range input {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// The tagged Mojo runtime input planner requires the exact
		// decoded type suffix, without trimming the type's trailing spaces.
		// A call ID is independently accepted only when nonblank.
		kind, _ := object["type"].(string)
		callID := nonblankJSONText(object["call_id"])
		if callID != "" && strings.HasSuffix(kind, "_call_output") {
			return true
		}
	}
	return false
}
