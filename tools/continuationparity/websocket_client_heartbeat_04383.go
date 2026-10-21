package main

import (
	"bufio"
	"fmt"
	"net"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

// The 0.438.4 liveness plan sends transport-only empty Ping control frames
// every 1000ms while waiting upstream. Clients answer them with masked Pong,
// but neither Ping nor Pong counts as committed output or upstream progress.
// Never skip unexpected application data, invalid control frames or CLOSE.
func readWebSocketLivenessFrame04383(connection net.Conn, reader *bufio.Reader) (byte, []byte, int, error) {
	const maxControlFrames = 48 // bounded even for the ~30-second deadline case
	heartbeats := 0
	var pongErr error
	for heartbeats < maxControlFrames {
		frame, err := websocketframe.ReadHeader(reader)
		if err != nil {
			return 0, nil, heartbeats, err
		}
		if frame.Masked() || !frame.Final || frame.Header[0]&0x70 != 0 || frame.PayloadLength > 1<<20 {
			return 0, nil, heartbeats, fmt.Errorf("malformed upstream WebSocket frame: opcode=%d len=%d", frame.Opcode, frame.PayloadLength)
		}
		payload, err := frame.ReadPayload(reader, 1<<20)
		if err != nil {
			return 0, nil, heartbeats, err
		}
		switch frame.Opcode {
		case 9:
			if pongErr != nil {
				return 0, nil, heartbeats, pongErr
			}
			if len(payload) != 0 {
				return 0, nil, heartbeats, fmt.Errorf("heartbeat Ping unexpectedly carried %d bytes", len(payload))
			}
			heartbeats++
			// A closing peer can reject Pong while its complete CLOSE is queued.
			pongErr = websocketframe.WriteFrame(connection, 10, payload, true)
		case 1:
			if pongErr != nil {
				return 0, nil, heartbeats, pongErr
			}
			return frame.Opcode, payload, heartbeats, nil
		case 8:
			return frame.Opcode, payload, heartbeats, nil
		default:
			return 0, nil, heartbeats, fmt.Errorf("unexpected WebSocket opcode during liveness: %d", frame.Opcode)
		}
	}
	return 0, nil, heartbeats, fmt.Errorf("more than %d heartbeat Pings without an application frame", maxControlFrames)
}
