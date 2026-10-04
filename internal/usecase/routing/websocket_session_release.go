package routing

type websocketMessageSessionReleaser interface {
	CloseWebSocketSession(uint64)
}

func (router *Router) ReleaseWebSocketMessageSession(sessionID uint64) {
	if sessionID == 0 || router == nil || router.gateway == nil {
		return
	}
	if closer, ok := router.gateway.(websocketMessageSessionReleaser); ok {
		closer.CloseWebSocketSession(sessionID)
	}
}
