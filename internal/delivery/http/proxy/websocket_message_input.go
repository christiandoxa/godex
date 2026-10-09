package proxy

import (
	"bufio"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const (
	websocketDefaultMaxFrameBytes   uint64 = 16 << 20
	websocketDefaultMaxMessageBytes uint64 = 64 << 20
)

type websocketInputKind uint8

const (
	websocketInputText websocketInputKind = iota
	websocketInputBinary
	websocketInputClose
	websocketInputControl
)

type websocketProtocolLimits struct {
	frameBytes   uint64
	messageBytes uint64
}

var defaultWebSocketProtocolLimits = websocketProtocolLimits{
	frameBytes:   websocketDefaultMaxFrameBytes,
	messageBytes: websocketDefaultMaxMessageBytes,
}

func readWebSocketClientMessage(
	reader *bufio.Reader,
	writer *websocketFrameWriter,
) ([]byte, websocketInputKind, error) {
	return readWebSocketClientMessageWithLimits(reader, writer, defaultWebSocketProtocolLimits)
}

func readWebSocketClientMessageWithLimits(
	reader *bufio.Reader,
	writer *websocketFrameWriter,
	limits websocketProtocolLimits,
) ([]byte, websocketInputKind, error) {
	if limits.frameBytes == 0 || limits.messageBytes == 0 {
		return nil, websocketInputText, errors.New("websocket protocol limits must be positive")
	}
	var payload []byte
	var messageBytes uint64
	var kind websocketInputKind
	var started, fragmented bool
	var initialOpcode byte
	for {
		remaining := limits.messageBytes - messageBytes
		frame, err := readClientFrame(reader, writer, limits.frameBytes, remaining)
		if err != nil {
			return nil, websocketInputText, err
		}
		if frame.kind == websocketInputClose {
			return nil, websocketInputClose, nil
		}
		if frame.kind == websocketInputControl {
			continue
		}
		messageBytes += frame.payloadLength
		if !started {
			if frame.opcode != 1 && frame.opcode != 2 {
				return nil, websocketInputText, errors.New("unexpected websocket continuation frame")
			}
			started, fragmented, initialOpcode = true, !frame.final, frame.opcode
			if initialOpcode == 2 {
				kind = websocketInputBinary
			}
		} else {
			if frame.opcode != 0 || !fragmented {
				return nil, websocketInputText, errors.New("invalid fragmented websocket message")
			}
			fragmented = !frame.final
		}
		if frame.payload != nil && kind != websocketInputBinary {
			payload = append(payload, frame.payload...)
		}
		if !frame.final {
			continue
		}
		if kind == websocketInputBinary {
			return nil, websocketInputBinary, nil
		}
		if !utf8.Valid(payload) {
			return nil, websocketInputText, errors.New("websocket text message is not valid UTF-8")
		}
		return payload, websocketInputText, nil
	}
}

type websocketClientFrame struct {
	opcode        byte
	final         bool
	payload       []byte
	payloadLength uint64
	kind          websocketInputKind
}

func readClientFrame(
	reader *bufio.Reader,
	writer *websocketFrameWriter,
	maxFrameBytes uint64,
	remainingMessageBytes uint64,
) (websocketClientFrame, error) {
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		return websocketClientFrame{}, err
	}
	if frame.Header[0]&0x70 != 0 || !frame.Masked() {
		return websocketClientFrame{}, errors.New("invalid client websocket frame")
	}
	if frame.PayloadLength > maxFrameBytes {
		return websocketClientFrame{}, errors.New("websocket frame exceeds protocol size limit")
	}
	if frame.Opcode >= 8 {
		return readClientControlFrame(reader, writer, frame)
	}
	if frame.Opcode != 0 && frame.Opcode != 1 && frame.Opcode != 2 {
		return websocketClientFrame{}, errors.New("unsupported websocket data frame")
	}
	if frame.PayloadLength > remainingMessageBytes {
		return websocketClientFrame{}, errors.New("websocket message exceeds protocol size limit")
	}
	kind := websocketInputText
	if frame.Opcode == 2 {
		kind = websocketInputBinary
	}
	if kind == websocketInputBinary || frame.Opcode == 0 {
		if kind == websocketInputBinary {
			if _, err := io.CopyN(io.Discard, reader, int64(frame.PayloadLength)); err != nil {
				return websocketClientFrame{}, err
			}
			return websocketClientFrame{
				opcode: frame.Opcode, final: frame.Final,
				payloadLength: frame.PayloadLength, kind: kind,
			}, nil
		}
	}
	payload, err := frame.ReadPayload(reader, frame.PayloadLength)
	if err != nil {
		return websocketClientFrame{}, err
	}
	frame.Unmask(payload)
	return websocketClientFrame{
		opcode: frame.Opcode, final: frame.Final, payload: payload,
		payloadLength: frame.PayloadLength, kind: kind,
	}, nil
}

func readClientControlFrame(
	reader *bufio.Reader,
	writer *websocketFrameWriter,
	frame websocketframe.Frame,
) (websocketClientFrame, error) {
	if !frame.Final || frame.PayloadLength > 125 {
		return websocketClientFrame{}, errors.New("invalid websocket control frame")
	}
	payload, err := frame.ReadPayload(reader, 125)
	if err != nil {
		return websocketClientFrame{}, err
	}
	frame.Unmask(payload)
	switch frame.Opcode {
	case 8:
		payload, err = normalizeWebSocketClosePayload(payload)
		if err != nil {
			return websocketClientFrame{}, err
		}
		return websocketClientFrame{kind: websocketInputClose}, writer.writeFrame(8, payload)
	case 9:
		if err := writer.writeFrame(10, payload); err != nil {
			return websocketClientFrame{}, err
		}
		return websocketClientFrame{kind: websocketInputControl}, nil
	case 10:
		return websocketClientFrame{kind: websocketInputControl}, nil
	default:
		return websocketClientFrame{}, errors.New("unsupported websocket control frame")
	}
}

func normalizeWebSocketClosePayload(payload []byte) ([]byte, error) {
	return websocketframe.NormalizeClosePayload(payload)
}
