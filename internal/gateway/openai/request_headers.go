package openai

import (
	"github.com/christiandoxa/godex/internal/helper/httpheader"
	"net/http"
	"strings"
)

func removeHopHeaders(headers http.Header) {
	connectionHeaders := httpheader.ConnectionTokens(headers)
	for key := range headers {
		name := strings.ToLower(strings.TrimSpace(key))
		if httpheader.IsRequestTransport(name) || connectionHeaders[http.CanonicalHeaderKey(key)] ||
			strings.HasPrefix(name, "sec-websocket-") ||
			strings.HasPrefix(name, "x-prodex-internal-") ||
			name == "authorization" || name == "chatgpt-account-id" {
			delete(headers, key)
		}
	}
}

func prepareWebSocketRequestHeaders(headers http.Header, websocketKey string) {
	removeHopHeaders(headers)
	headers.Set("Sec-WebSocket-Key", websocketKey)
	headers.Set("Sec-WebSocket-Version", "13")
	headers.Set("Connection", "Upgrade")
	headers.Set("Upgrade", "websocket")
}
