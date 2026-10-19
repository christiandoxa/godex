package proxy

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04383LateWebSocketResponseOwnerPersistsBeforeTerminalDelivery(t *testing.T) {
	for _, mode := range []string{"plain", "fragmented", "large envelope"} {
		t.Run(mode, func(t *testing.T) {
			accounts := testRuntimeAccounts(t, "A", "synthetic-a", "B", "synthetic-b")[:1]
			accounts[0].ID = strings.Repeat("a", 32)
			store := routingrepo.NewStore(t.TempDir())
			padding := ""
			if mode == "large envelope" {
				padding = strings.Repeat("x", 128<<10)
			}
			terminal := fmt.Sprintf(`{"type":"response.completed","response":{"padding":%q,"id":"resp-late"}}`, padding)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, buffered, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				digest := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
				_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(digest[:]))
				if buffered.Flush() != nil {
					return
				}
				frame, err := websocketframe.ReadHeader(buffered.Reader)
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := frame.ReadPayload(buffered.Reader, 1<<20); err != nil {
					t.Error(err)
					return
				}
				if err := websocketframe.WriteFrame(conn, 1, []byte(`{"type":"response.output_text.delta","delta":"visible"}`), false); err != nil {
					t.Error(err)
					return
				}
				if mode == "fragmented" {
					_, _ = conn.Write(append([]byte{0x01, 20}, []byte(terminal[:20])...))
					_, _ = conn.Write(append([]byte{0x80, byte(len(terminal) - 20)}, []byte(terminal[20:])...))
				} else if err := websocketframe.WriteFrame(conn, 1, []byte(terminal), false); err != nil {
					t.Error(err)
				}
			}))
			defer upstream.Close()
			proxy, err := newProxyForTest(ProxyConfig{UpstreamURL: upstream.URL, Bindings: store, Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil }})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(proxy)
			defer server.Close()
			defer proxy.router.Close()
			conn, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/prodex/responses")
			defer conn.Close()
			if status, _ := readResponsesPublicHandshake(t, reader); status != 101 {
				t.Fatalf("handshake: %d", status)
			}
			if err := websocketframe.WriteFrame(conn, 1, []byte(`{"type":"response.create","input":[]}`), true); err != nil {
				t.Fatal(err)
			}
			if got := readResponsesPublicTextFrame(t, reader); got != `{"type":"response.output_text.delta","delta":"visible"}` {
				t.Fatalf("changed delta: %s", got)
			}
			var received strings.Builder
			for received.Len() < len(terminal) {
				frame, err := websocketframe.ReadHeader(reader)
				if err != nil {
					t.Fatal(err)
				}
				payload, err := frame.ReadPayload(reader, 1<<20)
				if err != nil {
					t.Fatal(err)
				}
				if frame.Masked() || frame.Opcode != 1 && frame.Opcode != 0 {
					t.Fatalf("invalid terminal frame: %+v", frame)
				}
				received.Write(payload)
			}
			if received.String() != terminal {
				t.Fatal("changed terminal payload")
			}
			bindings, err := store.Load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte("previous:resp-late"))
			for _, binding := range bindings {
				if binding.Key == hex.EncodeToString(digest[:]) && binding.AccountID == accounts[0].ID {
					return
				}
			}
			t.Fatalf("client received terminal response before its durable owner: %v", bindings)
		})
	}
}
