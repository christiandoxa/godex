package proxy

import (
	"bufio"
	"errors"
	"io"
	"sync"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

type realtimeUpstreamWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (writer *realtimeUpstreamWriter) writeMessage(opcode byte, payload []byte) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return websocketframe.WriteFrame(writer.writer, opcode, payload, true)
}

func runRealtimeWebSocketDuplex(
	clientReader *bufio.Reader,
	toClient *websocketFrameWriter,
	upstream io.ReadWriteCloser,
	stop func(),
) error {
	upstreamWriter := &realtimeUpstreamWriter{writer: upstream}
	results := make(chan error, 2)
	go func() {
		results <- pumpRealtimeClient(clientReader, toClient, upstreamWriter)
	}()
	go func() {
		results <- pumpRealtimeUpstream(upstream, toClient, upstreamWriter)
	}()
	first := <-results
	stop()
	second := <-results
	if first != nil {
		return first
	}
	return second
}

func pumpRealtimeClient(
	reader *bufio.Reader,
	toClient *websocketFrameWriter,
	upstream *realtimeUpstreamWriter,
) error {
	for {
		opcode, payload, closeMessage, err := readRealtimeMessage(reader, true, func(opcode byte, payload []byte) error {
			switch opcode {
			case 9:
				return toClient.writeFrame(10, payload)
			case 10:
				return nil
			case 8:
				if err := upstream.writeMessage(8, payload); err != nil {
					return err
				}
				return toClient.writeFrame(8, payload)
			default:
				return errors.New("unsupported realtime client control frame")
			}
		})
		if err != nil {
			return err
		}
		if closeMessage {
			return nil
		}
		if err := upstream.writeMessage(opcode, payload); err != nil {
			return err
		}
	}
}

func pumpRealtimeUpstream(
	reader io.Reader,
	toClient *websocketFrameWriter,
	upstream *realtimeUpstreamWriter,
) error {
	for {
		opcode, payload, closeMessage, err := readRealtimeMessage(reader, false, func(opcode byte, payload []byte) error {
			switch opcode {
			case 9:
				return upstream.writeMessage(10, payload)
			case 10:
				return nil
			case 8:
				return toClient.writeFrame(8, payload)
			default:
				return errors.New("unsupported realtime upstream control frame")
			}
		})
		if err != nil {
			return err
		}
		if closeMessage {
			return nil
		}
		if err := toClient.writeFrame(opcode, payload); err != nil {
			return err
		}
	}
}

func readRealtimeMessage(
	reader io.Reader,
	expectMasked bool,
	control func(byte, []byte) error,
) (byte, []byte, bool, error) {
	var messageOpcode byte
	var payload []byte
	var messageBytes uint64
	started := false
	for {
		frame, err := websocketframe.ReadHeader(reader)
		if err != nil {
			return 0, nil, false, err
		}
		if frame.Header[0]&0x70 != 0 || frame.Masked() != expectMasked {
			return 0, nil, false, errors.New("invalid realtime websocket frame")
		}
		if frame.PayloadLength > websocketDefaultMaxFrameBytes {
			return 0, nil, false, errors.New("realtime websocket frame exceeds protocol size limit")
		}
		if frame.Opcode >= 8 {
			if !frame.Final || frame.PayloadLength > 125 {
				return 0, nil, false, errors.New("invalid realtime websocket control frame")
			}
			controlPayload, err := frame.ReadPayload(reader, 125)
			if err != nil {
				return 0, nil, false, err
			}
			frame.Unmask(controlPayload)
			if frame.Opcode == 8 {
				controlPayload, err = normalizeWebSocketClosePayload(controlPayload)
				if err != nil {
					return 0, nil, false, err
				}
			}
			if err := control(frame.Opcode, controlPayload); err != nil {
				return 0, nil, false, err
			}
			if frame.Opcode == 8 {
				return 0, nil, true, nil
			}
			continue
		}
		if !started {
			if frame.Opcode != 1 && frame.Opcode != 2 {
				return 0, nil, false, errors.New("unexpected realtime websocket continuation frame")
			}
			started = true
			messageOpcode = frame.Opcode
		} else if frame.Opcode != 0 {
			return 0, nil, false, errors.New("invalid fragmented realtime websocket message")
		}
		if messageBytes > websocketDefaultMaxMessageBytes-frame.PayloadLength {
			return 0, nil, false, errors.New("realtime websocket message exceeds protocol size limit")
		}
		messageBytes += frame.PayloadLength
		chunk, err := frame.ReadPayload(reader, frame.PayloadLength)
		if err != nil {
			return 0, nil, false, err
		}
		frame.Unmask(chunk)
		payload = append(payload, chunk...)
		if !frame.Final {
			continue
		}
		if messageOpcode == 1 && !utf8.Valid(payload) {
			return 0, nil, false, errors.New("realtime websocket text message is not valid UTF-8")
		}
		return messageOpcode, payload, false, nil
	}
}
