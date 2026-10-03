package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebSocketIngressMatchesProdexUpgradeDetection(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://example.test/backend-api/codex/responses", nil)
	request.Header.Add("Upgrade", "h2c")
	request.Header.Add("Upgrade", "WebSocket")
	request.Header.Set("Sec-WebSocket-Key", "not-base64-and-still-valid-for-local-accept")
	if !isWebSocketUpgradeRequest(request) {
		t.Fatal("exact websocket Upgrade header was not detected")
	}
	if status, message := websocketRequestError(request); status != 0 {
		t.Fatalf("Prodex-compatible local handshake rejected: %d %q", status, message)
	}

	notWebSocket := httptest.NewRequest(http.MethodGet, "http://example.test/backend-api/codex/responses", nil)
	notWebSocket.Header.Set("Upgrade", "h2c, websocket")
	if isWebSocketUpgradeRequest(notWebSocket) {
		t.Fatal("comma-combined non-exact Upgrade value was treated as websocket")
	}
}

func TestSupportedWebSocketPathsMatchProdexRuntimePlanner(t *testing.T) {
	for _, path := range []string{
		"/backend-api/codex/responses",
		"/backend-api/prodex/responses",
		"/backend-api/prodex/v0.2.99/responses",
		"/backend-api/prodex/victim/realtime",
		"/backend-api/codex/nested/realtime",
		"/backend-api/codex/live",
		"/backend-api/codex/live/call-123",
		"/backend-api/prodex/v0.2.99/live/call-123",
		"/v1/realtime",
		"/v1/live",
		"/other/live/call-123",
	} {
		if !supportedWebSocketPath(path) {
			t.Errorf("Prodex-supported websocket path rejected: %s", path)
		}
	}
	for _, path := range []string{
		"/responses",
		"/v1/responses",
		"/backend-api/prodex/v0.2.99/realtime/calls",
		"/backend-api/codex/live/call-123/extra",
		"/backend-api/codex/responses/compact",
		"/unrelated",
	} {
		if supportedWebSocketPath(path) {
			t.Errorf("Prodex-unsupported websocket path accepted: %s", path)
		}
	}
}
