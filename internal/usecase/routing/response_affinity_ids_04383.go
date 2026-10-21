package routing

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

// The tagged 0.438.3 Mojo metadata planner independently records up to
// three response IDs in each JSON object, including duplicate-free aliases.
// Only those verified IDs may become hard continuation owners.
func responseAffinityWithIDs(headers http.Header, body []byte, stream bool) (affinityKeys, []string) {
	keys := affinityKeys{
		thread:  firstHeader(headers, "thread-id", "x-codex-thread-id"),
		turn:    strings.TrimSpace(headers.Get("x-codex-turn-state")),
		session: firstHeader(headers, "session_id", "session-id", "x-session-id"),
	}
	if len(body) == 0 {
		return keys, nil
	}
	var ids []string
	addEvent := func(data []byte) {
		if !validNativeJSONUnicode04383(data) {
			return
		}
		var object map[string]any
		if json.Unmarshal(data, &object) != nil || !validNativeJSONContainerDepth04383(object) {
			return
		}
		keys = mergeAffinityKeys(keys, responseObjectAffinity(object))
		ids = appendResponseObjectIDs04383(ids, object)
	}
	if stream {
		decoder := sse.NewDecoder(len(body))
		events := decoder.Feed(body)
		events = append(events, decoder.Finish()...)
		for _, data := range events {
			addEvent(data)
		}
	} else {
		addEvent(body)
	}
	return keys, ids
}

func appendResponseObjectIDs04383(ids []string, object map[string]any) []string {
	response, _ := object["response"].(map[string]any)
	candidates := [3]string{
		objectString(response, "id"),
		objectString(object, "response_id"),
	}
	if responseKindAdmitsRootID04383(object) {
		candidates[2] = objectString(object, "id")
	}
	for _, candidate := range candidates {
		if candidate != "" && len(ids) < affinityMaxValues && !slices.Contains(ids, candidate) {
			ids = append(ids, candidate)
		}
	}
	return ids
}
