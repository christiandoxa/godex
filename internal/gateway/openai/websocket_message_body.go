package openai

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

type websocketMessageState struct {
	turnState string
}

type websocketResponseBody struct {
	connection io.ReadWriteCloser
	prefix     []byte
	offset     int

	state   *websocketMessageState
	recycle func(io.ReadWriteCloser, string)

	messageOpcode byte
	messageOpen   bool
	messageBytes  uint64
	messageBody   []byte

	frameActive    bool
	frameFinal     bool
	frameRemaining uint64

	done             bool
	terminalObserved bool
	terminalReset    bool
	closeOnce        sync.Once
	closeErr         error
}

func newWebSocketResponseBody(
	connection io.ReadWriteCloser,
	prefix []byte,
	terminalKind string,
	state *websocketMessageState,
	recycle func(io.ReadWriteCloser, string),
) *websocketResponseBody {
	body := &websocketResponseBody{
		connection: connection,
		prefix:     prefix,
		state:      state,
		recycle:    recycle,
	}
	if terminalKind != "" {
		body.done = true
		body.terminalObserved = true
		body.terminalReset = websocketTerminalShouldReset(terminalKind)
	}
	return body
}

func (body *websocketResponseBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	for {
		if body.offset < len(body.prefix) {
			count := copy(buffer, body.prefix[body.offset:])
			body.offset += count
			return count, nil
		}
		if body.done {
			return 0, io.EOF
		}
		if body.frameActive {
			if body.frameRemaining > 0 {
				limit := min(uint64(len(buffer)), body.frameRemaining)
				count, err := body.connection.Read(buffer[:int(limit)])
				if count > 0 {
					body.frameRemaining -= uint64(count)
					if body.messageOpcode == 1 {
						body.messageBody = append(body.messageBody, buffer[:count]...)
					}
					return count, nil
				}
				if err != nil {
					return 0, websocketUnexpectedEOF(err)
				}
				return 0, io.ErrNoProgress
			}
			if err := body.finishFrame(); err != nil {
				return 0, err
			}
			continue
		}
		if err := body.startFrame(); err != nil {
			return 0, err
		}
	}
}

func (body *websocketResponseBody) Write(payload []byte) (int, error) {
	return body.connection.Write(payload)
}

func (body *websocketResponseBody) startFrame() error {
	frame, err := websocketframe.ReadHeader(body.connection)
	if err != nil {
		return websocketUnexpectedEOF(err)
	}
	if frame.Header[0]&0x70 != 0 || frame.Masked() {
		return errors.New("invalid upstream websocket server frame")
	}
	if frame.PayloadLength > websocketUpstreamMaxFrameBytes {
		return errors.New("upstream websocket frame exceeds protocol size limit")
	}
	if frame.Opcode >= 8 {
		return body.handleControlFrame(frame)
	}
	switch frame.Opcode {
	case 1:
		if body.messageOpen {
			return errors.New("invalid fragmented upstream websocket message")
		}
		body.messageOpcode = 1
		body.messageOpen = !frame.Final
		body.messageBytes = 0
		body.messageBody = body.messageBody[:0]
	case 2:
		if body.messageOpen {
			return errors.New("invalid fragmented upstream websocket message")
		}
		body.messageOpcode = 2
		body.messageOpen = !frame.Final
		body.messageBytes = 0
		body.messageBody = body.messageBody[:0]
	case 0:
		if !body.messageOpen {
			return errors.New("unexpected upstream websocket continuation frame")
		}
		body.messageOpen = !frame.Final
	default:
		return errors.New("unsupported upstream websocket data frame")
	}
	if body.messageBytes > websocketUpstreamMaxMessageBytes-frame.PayloadLength {
		return errors.New("upstream websocket message exceeds protocol size limit")
	}
	body.messageBytes += frame.PayloadLength
	body.prefix = append(body.prefix[:0], frame.Header...)
	body.offset = 0
	body.frameActive = true
	body.frameFinal = frame.Final
	body.frameRemaining = frame.PayloadLength
	return nil
}

func (body *websocketResponseBody) finishFrame() error {
	body.frameActive = false
	if !body.frameFinal {
		return nil
	}
	if body.messageOpcode == 1 {
		if !utf8.Valid(body.messageBody) {
			return errors.New("upstream websocket text message is not valid UTF-8")
		}
		kind, _, turnState := websocketEventMetadata(body.messageBody)
		if turnState != "" && body.state != nil {
			body.state.turnState = turnState
		}
		if websocketPayloadTerminal(kind, body.messageBody) {
			body.done = true
			body.terminalObserved = true
			body.terminalReset = websocketTerminalShouldReset(kind)
		}
	}
	body.messageOpcode = 0
	body.messageOpen = false
	body.messageBytes = 0
	body.messageBody = body.messageBody[:0]
	return nil
}

func (body *websocketResponseBody) handleControlFrame(frame websocketframe.Frame) error {
	if !frame.Final || frame.PayloadLength > 125 {
		return errors.New("invalid upstream websocket control frame")
	}
	payload, err := frame.ReadPayload(body.connection, 125)
	if err != nil {
		return err
	}
	if frame.Opcode == 8 {
		payload, err = websocketframe.NormalizeClosePayload(payload)
		if err != nil {
			return err
		}
	} else if frame.Opcode != 9 && frame.Opcode != 10 {
		return errors.New("unsupported upstream websocket control frame")
	}
	var encoded bytes.Buffer
	if err := websocketframe.WriteFrame(&encoded, frame.Opcode, payload, false); err != nil {
		return err
	}
	body.prefix = append(body.prefix[:0], encoded.Bytes()...)
	body.offset = 0
	body.done = frame.Opcode == 8
	return nil
}

func websocketTerminalShouldReset(kind string) bool {
	return kind == "error" || kind == "response.failed" || kind == "response.incomplete"
}

func websocketUnexpectedEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

func (body *websocketResponseBody) Close() error {
	body.closeOnce.Do(func() {
		if body.terminalObserved && !body.terminalReset && body.recycle != nil {
			turnState := ""
			if body.state != nil {
				turnState = body.state.turnState
			}
			body.recycle(body.connection, turnState)
			return
		}
		body.closeErr = body.connection.Close()
	})
	return body.closeErr
}
