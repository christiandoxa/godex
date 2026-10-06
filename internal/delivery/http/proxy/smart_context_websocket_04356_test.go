package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProdex04356SmartContextWebSocketTransportGuards(t *testing.T) {
	long := strings.Repeat("websocket duplicate context ", 450)
	body := smartContextFixture("gpt-5.4", []any{
		messageInput("user", long),
		messageInput("user", long),
	})
	got := prepareSmartContextWebSocketBody(true, "/backend-api/godex/responses", nil, body)
	if !got.Rewritten || !bytes.Contains(got.Body, []byte("[godex-context-ref ")) {
		t.Fatalf("eligible websocket body was not rewritten: rewritten=%t", got.Rewritten)
	}

	var create map[string]any
	if err := json.Unmarshal(body, &create); err != nil {
		t.Fatal(err)
	}
	create["type"] = "response.create"
	create["generate"] = false
	generateFalse, err := json.Marshal(create)
	if err != nil {
		t.Fatal(err)
	}
	got = prepareSmartContextWebSocketBody(true, "/backend-api/godex/responses", nil, generateFalse)
	if got.Rewritten || !bytes.Equal(got.Body, generateFalse) {
		t.Fatal("response.create generate=false was rewritten")
	}

	headers := http.Header{"X-Godex-Smart-Context": []string{"exact"}}
	got = prepareSmartContextWebSocketBody(true, "/backend-api/godex/responses", headers, body)
	if got.Rewritten || !bytes.Equal(got.Body, body) {
		t.Fatal("websocket exact-mode body changed")
	}

	oversizedText := strings.Repeat("x", smartContextWebSocketRewriteMaxBytes)
	oversized := smartContextFixture("gpt-5.4", []any{
		messageInput("user", oversizedText),
		messageInput("user", oversizedText),
	})
	if len(oversized) <= smartContextWebSocketRewriteMaxBytes {
		t.Fatalf("oversized fixture = %d", len(oversized))
	}
	got = prepareSmartContextWebSocketBody(true, "/backend-api/godex/responses", nil, oversized)
	if got.Rewritten || !bytes.Equal(got.Body, oversized) {
		t.Fatal("oversized websocket body changed")
	}
}

func TestProdex04356SmartContextPublicResponsesWebSocketRewritesBeforeRouting(t *testing.T) {
	gateway := &responsesPublicGateway{closed: make(chan uint64, 1)}
	proxy := newResponsesPublicProxy(t, gateway)
	proxy.smartContextEnabled = true
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/codex/responses")
	defer connection.Close()
	status, _ := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d", status)
	}

	long := strings.Repeat("websocket routed duplicate ", 500)
	value := map[string]any{
		"type":  "response.create",
		"model": "gpt-5.4",
		"input": []any{
			messageInput("user", long),
			messageInput("user", long),
		},
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Write(protocolClientFrame(1, true, payload, true)); err != nil {
		t.Fatal(err)
	}
	_ = readResponsesPublicTextFrame(t, reader)

	gateway.mu.Lock()
	bodies := append([]string(nil), gateway.bodies...)
	gateway.mu.Unlock()
	if len(bodies) != 1 ||
		!strings.Contains(bodies[0], "[godex-context-ref ") ||
		!strings.Contains(bodies[0], smartContextInlineReferenceProtocol) {
		t.Fatalf("gateway websocket body = %#v", bodies)
	}
	if bodies[0] == string(payload) {
		t.Fatal("websocket gateway received original body")
	}
}
