package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

func TestProxyWebSocketTranslatesKnownOwnerPreviousResponseFailureToStaleContinuation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", websocketAccept(request.Header.Get("Sec-WebSocket-Key")))
		if err := buffered.Flush(); err != nil {
			t.Errorf("flush upstream handshake: %v", err)
			return
		}
		for index, event := range []string{
			`{"type":"response.completed","response":{"id":"resp_original"}}`,
			`{"type":"response.failed","response":{"id":"resp_missing","error":{"type":"invalid_request_error","code":"previous_response_not_found","message":"missing"}}}`,
		} {
			opcode, payload, err := readWebSocketTestFrame(buffered.Reader)
			if err != nil || opcode != 1 {
				t.Errorf("read upstream request %d: opcode %d, error %v", index+1, opcode, err)
				return
			}
			if index == 1 && !strings.Contains(string(payload), `"previous_response_id":"resp_original"`) {
				t.Errorf("continuation request = %q", payload)
				return
			}
			if err := websocketframe.WriteFrame(buffered, 1, []byte(event), false); err != nil {
				t.Errorf("write upstream response %d: %v", index+1, err)
				return
			}
			if err := buffered.Flush(); err != nil {
				t.Errorf("flush upstream response %d: %v", index+1, err)
				return
			}
		}
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, testRuntimeAccounts(t, "A", "synthetic-token-a", "B", "synthetic-token-b"))

	connection, reader := dialWebSocket(t, proxy.URL, "/backend-api/codex/responses", "")
	defer connection.Close()
	status, _, _ := readWebSocketHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("local websocket status = %d", status)
	}
	_ = connection.SetDeadline(time.Now().Add(3 * time.Second))

	for _, request := range []string{
		`{"type":"response.create","response":{}}`,
		`{"type":"response.create","response":{"previous_response_id":"resp_original"}}`,
	} {
		if _, err := connection.Write(maskedWebSocketFrame(1, true, request)); err != nil {
			t.Fatalf("write client request: %v", err)
		}
		opcode, payload, err := readWebSocketTestFrame(reader)
		if err != nil || opcode != 1 {
			t.Fatalf("read client response: opcode %d, error %v", opcode, err)
		}
		if strings.Contains(request, "previous_response_id") {
			if !strings.Contains(string(payload), `"code":"stale_continuation"`) ||
				!strings.Contains(string(payload), `"status":409`) ||
				strings.Contains(string(payload), "previous_response_not_found") {
				t.Fatalf("stale continuation = %s", payload)
			}
		} else if !strings.Contains(string(payload), `"id":"resp_original"`) {
			t.Fatalf("initial response = %s", payload)
		}
	}
}
