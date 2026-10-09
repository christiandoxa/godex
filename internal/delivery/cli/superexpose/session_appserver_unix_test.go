//go:build linux

package superexpose

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

func TestAppServerReadTextRejectsMaskedAndReservedFrames(t *testing.T) {
	for _, test := range []struct {
		name   string
		masked bool
		rsv1   bool
	}{
		{name: "masked", masked: true},
		{name: "reserved", rsv1: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()
			socket := &appServerSocket{conn: client, reader: bufio.NewReader(client)}
			done := make(chan error, 1)
			go func() {
				frame := []byte{0x81, 0x01, 'x'}
				if test.rsv1 {
					frame[0] |= 0x40
				}
				if test.masked {
					frame[1] |= 0x80
					frame = append(frame[:2], 1, 2, 3, 4, 'y')
				}
				_, err := server.Write(frame)
				done <- err
			}()
			if _, err := socket.readText(); err == nil || !strings.Contains(err.Error(), "invalid app-server websocket") {
				t.Fatalf("read error = %v", err)
			}
			_ = server.Close()
			<-done
		})
	}
}

func TestAppServerReadTextAnswersPing(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	socket := &appServerSocket{conn: client, reader: bufio.NewReader(client)}
	done := make(chan struct{})
	go func() {
		_ = websocketframe.WriteFrame(server, 9, []byte("ping"), false)
		if frame, err := websocketframe.ReadHeader(server); err == nil {
			_, _ = frame.ReadPayload(server, 125)
		}
		_ = websocketframe.WriteFrame(server, 1, []byte("ok"), false)
		close(done)
	}()
	text, err := socket.readText()
	if err != nil || text != "ok" {
		t.Fatalf("read text = %q err=%v", text, err)
	}
	<-done
}

func TestAppServerThreadReadRejectsMalformedTurnIDs(t *testing.T) {
	for _, turnID := range []string{strings.Repeat("x", 129), "turn-" + string(rune(0x85))} {
		t.Run("malformed", func(t *testing.T) {
			server, client := net.Pipe()
			socket := &appServerSocket{conn: client, reader: bufio.NewReader(client)}
			root := t.TempDir()
			target := resolvedSessionTarget{
				threadID:    "thread-id",
				environment: targetEnvironment{pwd: root},
			}
			response := map[string]any{
				"id": 1,
				"result": map[string]any{"thread": map[string]any{
					"id": "thread-id", "sessionId": "thread-id", "ephemeral": false,
					"canAcceptDirectInput": true, "cwd": root,
					"status": map[string]any{"type": "active"},
					"turns": []any{map[string]any{
						"id": turnID, "status": map[string]any{"type": "inProgress"},
					}},
				}},
			}
			done := make(chan error, 1)
			go func() {
				frame, err := websocketframe.ReadHeader(server)
				if err == nil {
					_, err = frame.ReadPayload(server, appServerMaxMessageSize)
				}
				if err == nil {
					payload, marshalErr := json.Marshal(response)
					err = marshalErr
					if err == nil {
						err = websocketframe.WriteFrame(server, 1, payload, false)
					}
				}
				done <- err
			}()
			requestID := uint64(1)
			activity, err := appServerThreadRead(socket, target, true, &requestID)
			if activity != nil || !errors.Is(err, sessionVerificationInconclusive) {
				t.Fatalf("malformed turn ID accepted: activity=%#v err=%v", activity, err)
			}
			_ = server.Close()
			_ = client.Close()
			if err := <-done; err != nil && !errors.Is(err, net.ErrClosed) {
				t.Fatalf("fake app-server: %v", err)
			}
		})
	}
}
