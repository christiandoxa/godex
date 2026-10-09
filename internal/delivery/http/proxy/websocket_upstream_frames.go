package proxy

import (
	"errors"
	"io"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

func copyWebSocketFrames(reader io.Reader, writer *websocketFrameWriter) error {
	state := websocketServerForwardState{}
	for {
		frame, err := websocketframe.ReadHeader(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := forwardWebSocketServerFrame(reader, writer, frame, &state); err != nil {
			return err
		}
		if state.closed {
			return nil
		}
	}
}

type websocketServerForwardState struct {
	messageOpen  bool
	messageBytes uint64
	closed       bool
}

func forwardWebSocketServerFrame(
	reader io.Reader,
	writer *websocketFrameWriter,
	frame websocketframe.Frame,
	state *websocketServerForwardState,
) error {
	if frame.Header[0]&0x70 != 0 || frame.Masked() {
		return errors.New("invalid upstream websocket server frame")
	}
	if frame.PayloadLength > websocketDefaultMaxFrameBytes {
		return errors.New("upstream websocket frame exceeds protocol size limit")
	}
	if frame.Opcode >= 8 {
		return forwardWebSocketServerControl(reader, writer, frame, state)
	}
	if !state.messageOpen {
		if frame.Opcode != 1 && frame.Opcode != 2 {
			return errors.New("unexpected upstream websocket continuation frame")
		}
	} else if frame.Opcode != 0 {
		return errors.New("invalid fragmented upstream websocket message")
	}
	if state.messageBytes > websocketDefaultMaxMessageBytes-frame.PayloadLength {
		return errors.New("upstream websocket message exceeds protocol size limit")
	}
	state.messageBytes += frame.PayloadLength
	if err := writer.copyFrame(reader, frame); err != nil {
		return err
	}
	state.messageOpen = !frame.Final
	if frame.Final {
		state.messageBytes = 0
	}
	return nil
}

func forwardWebSocketServerControl(
	reader io.Reader,
	writer *websocketFrameWriter,
	frame websocketframe.Frame,
	state *websocketServerForwardState,
) error {
	if !frame.Final || frame.PayloadLength > 125 {
		return errors.New("invalid upstream websocket control frame")
	}
	payload, err := frame.ReadPayload(reader, 125)
	if err != nil {
		return err
	}
	switch frame.Opcode {
	case 9:
		connection, ok := reader.(io.Writer)
		if !ok {
			return errors.New("upstream websocket connection is not duplex")
		}
		return websocketframe.WriteFrame(connection, 10, payload, true)
	case 10:
		return nil
	case 8:
		payload, err = websocketframe.NormalizeClosePayload(payload)
		if err != nil {
			return err
		}
		connection, ok := reader.(io.Writer)
		if !ok {
			return errors.New("upstream websocket connection is not duplex")
		}
		if err := websocketframe.WriteFrame(connection, 8, payload, true); err != nil {
			return err
		}
		if err := writer.writeFrame(8, payload); err != nil {
			return err
		}
		state.closed = true
		return nil
	default:
		return errors.New("unsupported upstream websocket control frame")
	}
}
