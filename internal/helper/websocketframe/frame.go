package websocketframe

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"unicode/utf8"
)

type Frame struct {
	Header        []byte
	PayloadLength uint64
	Opcode        byte
	Final         bool
	maskKey       [4]byte
	masked        bool
}

func ReadHeader(reader io.Reader) (Frame, error) {
	var header [14]byte
	if _, err := io.ReadFull(reader, header[:2]); err != nil {
		return Frame{}, err
	}
	frame := Frame{Opcode: header[0] & 0x0f, Final: header[0]&0x80 != 0, masked: header[1]&0x80 != 0}
	length := uint64(header[1] & 0x7f)
	headerLen := 2
	switch length {
	case 126:
		if _, err := io.ReadFull(reader, header[headerLen:headerLen+2]); err != nil {
			return Frame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(header[headerLen : headerLen+2]))
		if length < 126 {
			return Frame{}, errors.New("websocket frame uses a non-minimal payload length")
		}
		headerLen += 2
	case 127:
		if _, err := io.ReadFull(reader, header[headerLen:headerLen+8]); err != nil {
			return Frame{}, err
		}
		length = binary.BigEndian.Uint64(header[headerLen : headerLen+8])
		if length>>63 != 0 {
			return Frame{}, errors.New("websocket frame length exceeds supported range")
		}
		if length < 1<<16 {
			return Frame{}, errors.New("websocket frame uses a non-minimal payload length")
		}
		headerLen += 8
	}
	if frame.masked {
		if _, err := io.ReadFull(reader, header[headerLen:headerLen+4]); err != nil {
			return Frame{}, err
		}
		copy(frame.maskKey[:], header[headerLen:headerLen+4])
		headerLen += 4
	}
	frame.Header = append([]byte(nil), header[:headerLen]...)
	frame.PayloadLength = length
	return frame, nil
}

// NormalizeClosePayload validates a close payload and maps a disallowed status
// code to the protocol-violation close used by the websocket implementations.
func NormalizeClosePayload(payload []byte) ([]byte, error) {
	switch len(payload) {
	case 0:
		return payload, nil
	case 1:
		return nil, errors.New("invalid websocket close sequence")
	}
	if !utf8.Valid(payload[2:]) {
		return nil, errors.New("websocket close reason is not valid UTF-8")
	}
	code := binary.BigEndian.Uint16(payload[:2])
	if WebSocketCloseCodeAllowed(code) {
		return payload, nil
	}
	normalized := make([]byte, 2, 2+len("Protocol violation"))
	binary.BigEndian.PutUint16(normalized, 1002)
	return append(normalized, "Protocol violation"...), nil
}

// WebSocketCloseCodeAllowed reports whether a close status is valid on the wire.
func WebSocketCloseCodeAllowed(code uint16) bool {
	switch code {
	case 1000, 1001, 1002, 1003, 1007, 1008, 1009, 1010, 1011, 1012, 1013:
		return true
	default:
		return code >= 3000 && code <= 4999
	}
}

func (frame Frame) Masked() bool { return frame.masked }

func (frame Frame) ReadPayload(reader io.Reader, max uint64) ([]byte, error) {
	if frame.PayloadLength > max || frame.PayloadLength > uint64(int(^uint(0)>>1)) {
		return nil, errors.New("websocket frame exceeds inspection limit")
	}
	payload := make([]byte, int(frame.PayloadLength))
	_, err := io.ReadFull(reader, payload)
	return payload, err
}

func (frame Frame) Unmask(payload []byte) {
	if !frame.masked {
		return
	}
	for index := range payload {
		payload[index] ^= frame.maskKey[index%len(frame.maskKey)]
	}
}

func (frame Frame) CopyTo(reader io.Reader, writer io.Writer) error {
	if err := writeAll(writer, frame.Header); err != nil {
		return err
	}
	_, err := io.CopyN(writer, reader, int64(frame.PayloadLength))
	return err
}

func (frame Frame) WriteTo(writer io.Writer, payload []byte) error {
	if uint64(len(payload)) != frame.PayloadLength {
		return errors.New("websocket frame payload length mismatch")
	}
	if err := writeAll(writer, frame.Header); err != nil {
		return err
	}
	return writeAll(writer, payload)
}

func WriteFrame(writer io.Writer, opcode byte, payload []byte, masked bool) error {
	var header [14]byte
	header[0] = 0x80 | opcode
	headerLen := 2
	switch length := len(payload); {
	case length < 126:
		header[1] = byte(length)
	case length <= 0xffff:
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:4], uint16(length))
		headerLen = 4
	default:
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:10], uint64(length))
		headerLen = 10
	}
	if masked {
		header[1] |= 0x80
		if _, err := rand.Read(header[headerLen : headerLen+4]); err != nil {
			return errors.New("generate websocket mask")
		}
		headerLen += 4
	}
	if err := writeAll(writer, header[:headerLen]); err != nil {
		return err
	}
	if !masked {
		return writeAll(writer, payload)
	}
	mask := header[headerLen-4 : headerLen]
	maskedPayload := make([]byte, len(payload))
	for index, value := range payload {
		maskedPayload[index] = value ^ mask[index%4]
	}
	return writeAll(writer, maskedPayload)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
