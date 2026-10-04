package routing

import (
	"encoding/json"
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
	var value map[string]any
	if json.Unmarshal(request.Body, &value) == nil {
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
	if direct := nonblankJSONText(value[directName]); direct != "" {
		return direct
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
		kind := nonblankJSONText(object["type"])
		callID := nonblankJSONText(object["call_id"])
		if callID != "" && strings.HasSuffix(kind, "_call_output") {
			return true
		}
	}
	return false
}
