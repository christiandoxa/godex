package proxy

import (
	"bufio"
	"errors"
	"io"
	"sync"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const websocketBinaryMessageError = `{"type":"error","status":400,"error":{"code":"invalid_request_error","message":"Binary websocket messages are not supported by the runtime auto-rotate proxy."}}`

type websocketFrameWriter struct {
	writer *bufio.Writer
	mu     sync.Mutex
}

func copyWebSocketClientFrames(reader io.Reader, upstream io.Writer, toClient *websocketFrameWriter) error {
	state := websocketClientForwardState{}
	for {
		frame, err := websocketframe.ReadHeader(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		err = forwardWebSocketClientFrame(reader, upstream, toClient, frame, &state)
		if err != nil {
			return err
		}
		if state.closed {
			return nil
		}
	}
}

type websocketClientForwardState struct {
	messageOpen   bool
	messageOpcode byte
	messageBytes  uint64
	textPayload   []byte
	closed        bool
}

func forwardWebSocketClientFrame(
	reader io.Reader,
	upstream io.Writer,
	toClient *websocketFrameWriter,
	frame websocketframe.Frame,
	state *websocketClientForwardState,
) error {
	if frame.Header[0]&0x70 != 0 || !frame.Masked() {
		return errors.New("invalid client websocket frame")
	}
	if frame.PayloadLength > websocketDefaultMaxFrameBytes {
		return errors.New("websocket frame exceeds protocol size limit")
	}
	if frame.Opcode >= 8 {
		return forwardWebSocketClientControl(reader, upstream, toClient, frame, state)
	}
	return forwardWebSocketClientData(reader, upstream, toClient, frame, state)
}

func forwardWebSocketClientControl(
	reader io.Reader,
	upstream io.Writer,
	toClient *websocketFrameWriter,
	frame websocketframe.Frame,
	state *websocketClientForwardState,
) error {
	if !frame.Final || frame.PayloadLength > 125 {
		return errors.New("invalid websocket control frame")
	}
	payload, err := frame.ReadPayload(reader, 125)
	if err != nil {
		return err
	}
	frame.Unmask(payload)
	switch frame.Opcode {
	case 8:
		payload, err = websocketframe.NormalizeClosePayload(payload)
		if err != nil {
			return err
		}
		if err := websocketframe.WriteFrame(upstream, 8, payload, true); err != nil {
			return err
		}
		if err := toClient.writeFrame(8, payload); err != nil {
			return err
		}
		state.closed = true
		return nil
	case 9:
		return toClient.writeFrame(10, payload)
	case 10:
		return nil
	default:
		return errors.New("unsupported websocket control frame")
	}
}

func forwardWebSocketClientData(
	reader io.Reader,
	upstream io.Writer,
	toClient *websocketFrameWriter,
	frame websocketframe.Frame,
	state *websocketClientForwardState,
) error {
	if !state.messageOpen {
		if frame.Opcode != 1 && frame.Opcode != 2 {
			return errors.New("unexpected websocket continuation frame")
		}
		state.messageOpcode = frame.Opcode
	} else if frame.Opcode != 0 {
		return errors.New("invalid fragmented websocket message")
	}
	if state.messageBytes > websocketDefaultMaxMessageBytes-frame.PayloadLength {
		return errors.New("websocket message exceeds protocol size limit")
	}
	state.messageBytes += frame.PayloadLength
	if state.messageOpcode == 2 {
		if _, err := io.CopyN(io.Discard, reader, int64(frame.PayloadLength)); err != nil {
			return err
		}
		state.messageOpen = !frame.Final
		if frame.Final {
			state.messageOpcode, state.messageBytes = 0, 0
			if err := toClient.writeText([]byte(websocketBinaryMessageError)); err != nil {
				return err
			}
		}
		return nil
	}

	payload, err := frame.ReadPayload(reader, frame.PayloadLength)
	if err != nil {
		return err
	}
	text := append([]byte(nil), payload...)
	frame.Unmask(text)
	state.textPayload = append(state.textPayload, text...)
	if frame.Final && !utf8.Valid(state.textPayload) {
		return errors.New("websocket text message is not valid UTF-8")
	}
	if err := frame.WriteTo(upstream, payload); err != nil {
		return err
	}
	state.messageOpen = !frame.Final
	if frame.Final {
		state.messageOpcode, state.messageBytes = 0, 0
		state.textPayload = state.textPayload[:0]
	}
	return nil
}

func (writer *websocketFrameWriter) copyFrame(reader io.Reader, frame websocketframe.Frame) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if err := frame.CopyTo(reader, writer.writer); err != nil {
		return err
	}
	return writer.writer.Flush()
}

func (writer *websocketFrameWriter) writeText(payload []byte) error {
	return writer.writeFrame(1, payload)
}

func (writer *websocketFrameWriter) writeFrame(opcode byte, payload []byte) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if err := websocketframe.WriteFrame(writer.writer, opcode, payload, false); err != nil {
		return err
	}
	return writer.writer.Flush()
}
