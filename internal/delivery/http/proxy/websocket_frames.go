package proxy

import (
	"bufio"
	"errors"
	"io"
	"sync"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const websocketBinaryMessageError = `{"type":"error","status":400,"error":{"code":"invalid_request_error","message":"Binary websocket messages are not supported by the runtime auto-rotate proxy."}}`

type websocketFrameWriter struct {
	writer *bufio.Writer
	mu     sync.Mutex
}

func copyWebSocketFrames(reader io.Reader, writer *websocketFrameWriter) error {
	for {
		frame, err := websocketframe.ReadHeader(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := writer.copyFrame(reader, frame); err != nil {
			return err
		}
	}
}

func copyWebSocketClientFrames(reader io.Reader, upstream io.Writer, toClient *websocketFrameWriter) error {
	var skippingBinary bool
	for {
		frame, err := websocketframe.ReadHeader(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		skippingBinary, err = forwardWebSocketClientFrame(reader, upstream, toClient, frame, skippingBinary)
		if err != nil {
			return err
		}
	}
}

func forwardWebSocketClientFrame(
	reader io.Reader,
	upstream io.Writer,
	toClient *websocketFrameWriter,
	frame websocketframe.Frame,
	skippingBinary bool,
) (bool, error) {
	if frame.Opcode == 9 && frame.PayloadLength <= 125 {
		payload, err := frame.ReadPayload(reader, 125)
		if err != nil {
			return skippingBinary, err
		}
		frame.Unmask(payload)
		return skippingBinary, toClient.writeFrame(10, payload)
	}

	wasSkippingBinary := skippingBinary
	binaryMessage := frame.Opcode == 2 || frame.Opcode == 0 && wasSkippingBinary
	switch frame.Opcode {
	case 2:
		skippingBinary = !frame.Final
	case 0:
		if wasSkippingBinary && frame.Final {
			skippingBinary = false
		}
	}
	if binaryMessage {
		if _, err := io.CopyN(io.Discard, reader, int64(frame.PayloadLength)); err != nil {
			return skippingBinary, err
		}
		if frame.Final && (frame.Opcode == 2 || wasSkippingBinary) {
			if err := toClient.writeText([]byte(websocketBinaryMessageError)); err != nil {
				return skippingBinary, err
			}
		}
		return skippingBinary, nil
	}
	if err := frame.CopyTo(reader, upstream); err != nil {
		return skippingBinary, err
	}
	return skippingBinary, nil
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
