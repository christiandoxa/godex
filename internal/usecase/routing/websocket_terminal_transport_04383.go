package routing

import (
	"encoding/json"
	"net/http"
)

// A typed transport attempt is empty until routing exhausts safe recovery.
// Finalize it here so delivery always receives a terminal client response.
func (pending *pendingResponse) finishWebSocketTransportFailure() {
	response := pending.response
	if response == nil || !response.WebSocketFrames || response.FirstEventCommitted ||
		response.WebSocketRealtimeDuplex || response.PrecommitFailure == nil || !response.PrecommitFailure.Transport {
		return
	}
	pending.close()
	payload, _ := json.Marshal(map[string]any{
		"type": "error", "status": http.StatusServiceUnavailable,
		"error": map[string]string{
			"code": "service_unavailable", "message": upstreamTransportFailureMessage,
		},
	})
	pending.response = websocketTextPayloadResponse(payload, http.StatusServiceUnavailable)
	pending.prefix = nil
}
