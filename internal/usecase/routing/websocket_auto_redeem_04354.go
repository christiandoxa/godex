package routing

import proxymodel "github.com/christiandoxa/godex/internal/model/proxy"

func freshAutoRedeemAllowedForResponse(
	request proxymodel.Request,
	pending *pendingResponse,
) bool {
	if !request.WebSocketMessage {
		return true
	}
	if pending == nil || pending.response == nil || pending.response.WebSocketFrames {
		return false
	}
	metadata := parseWebSocketRequestMetadata(request)
	return metadata.previousResponseID == "" &&
		metadata.sessionID == "" &&
		metadata.turnState == ""
}
