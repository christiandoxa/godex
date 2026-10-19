package main

import (
	"bufio"
	"bytes"
	"net"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

type failedPongConnection04383 struct{ net.Conn }

func (failedPongConnection04383) Write([]byte) (int, error) { return 0, net.ErrClosed }

func TestProdex04383LivenessReadsQueuedCloseAfterPongWriteFails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tail  []byte
		valid bool
	}{
		{"queued close", []byte{0x88, 0}, true},
		{"application data", []byte{0x81, 0}, false},
		{"another ping", []byte{0x89, 0}, false},
		{"abrupt EOF", nil, false},
		{"truncated close", []byte{0x88, 2, 3}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wire bytes.Buffer
			if err := websocketframe.WriteFrame(&wire, 9, nil, false); err != nil {
				t.Fatal(err)
			}
			wire.Write(tc.tail)
			opcode, payload, beats, err := readWebSocketLivenessFrame04383(failedPongConnection04383{}, bufio.NewReader(&wire))
			if tc.valid {
				if err != nil || opcode != 8 || len(payload) != 0 || beats != 1 {
					t.Fatalf("queued graceful close lost: opcode=%d payload=%q pings=%d err=%v", opcode, payload, beats, err)
				}
			} else if err == nil {
				t.Fatal("Pong failure accepted without a complete close frame")
			}
		})
	}
}
