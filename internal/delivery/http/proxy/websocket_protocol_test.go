package proxy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

func TestWebSocketProtocolDefaultsMatchTungstenite030(t *testing.T) {
	if websocketDefaultMaxFrameBytes != 16<<20 || websocketDefaultMaxMessageBytes != 64<<20 {
		t.Fatalf("websocket limits = frame %d message %d", websocketDefaultMaxFrameBytes, websocketDefaultMaxMessageBytes)
	}
}

func TestReadWebSocketClientMessageAcceptsFragmentedUTF8AcrossFrameBoundary(t *testing.T) {
	input := append(protocolClientFrame(1, false, []byte{0xe2}, true), protocolClientFrame(0, true, []byte{0x82, 0xac}, true)...)
	payload, kind, err := readProtocolMessage(t, input, websocketProtocolLimits{frameBytes: 4, messageBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocketInputText || string(payload) != "€" {
		t.Fatalf("message = kind %d payload %q", kind, payload)
	}
}

func TestReadWebSocketClientMessageRejectsInvalidUTF8(t *testing.T) {
	input := protocolClientFrame(1, true, []byte{0xff}, true)
	if _, _, err := readProtocolMessage(t, input, websocketProtocolLimits{frameBytes: 4, messageBytes: 8}); err == nil {
		t.Fatal("invalid UTF-8 text message was accepted")
	}
}

func TestReadWebSocketClientMessageEnforcesFrameAndMessageLimits(t *testing.T) {
	t.Run("frame", func(t *testing.T) {
		input := protocolClientFrame(1, true, []byte("12345"), true)
		if _, _, err := readProtocolMessage(t, input, websocketProtocolLimits{frameBytes: 4, messageBytes: 8}); err == nil {
			t.Fatal("oversized websocket frame was accepted")
		}
	})
	t.Run("fragmented message", func(t *testing.T) {
		input := append(protocolClientFrame(1, false, []byte("1234"), true), protocolClientFrame(0, true, []byte("567"), true)...)
		if _, _, err := readProtocolMessage(t, input, websocketProtocolLimits{frameBytes: 4, messageBytes: 6}); err == nil {
			t.Fatal("oversized fragmented websocket message was accepted")
		}
	})
	t.Run("binary message", func(t *testing.T) {
		input := append(protocolClientFrame(2, false, []byte("1234"), true), protocolClientFrame(0, true, []byte("567"), true)...)
		if _, _, err := readProtocolMessage(t, input, websocketProtocolLimits{frameBytes: 4, messageBytes: 6}); err == nil {
			t.Fatal("oversized binary websocket message was accepted")
		}
	})
}

func TestReadWebSocketClientMessageMatchesServerProtocolValidation(t *testing.T) {
	fixtures := []struct {
		name  string
		frame []byte
	}{
		{name: "unmasked", frame: protocolClientFrame(1, true, []byte("ok"), false)},
		{name: "reserved bit", frame: protocolClientFrameWithFirstByte(0xc1, []byte("ok"), true)},
		{name: "reserved data opcode", frame: protocolClientFrame(3, true, []byte("ok"), true)},
		{name: "reserved control opcode", frame: protocolClientFrame(11, true, nil, true)},
		{name: "fragmented control", frame: protocolClientFrame(9, false, []byte("x"), true)},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if _, _, err := readProtocolMessage(t, fixture.frame, websocketProtocolLimits{frameBytes: 16, messageBytes: 32}); err == nil {
				t.Fatal("invalid websocket frame was accepted")
			}
		})
	}
}

func TestReadWebSocketClientMessageRejectsMalformedClosePayload(t *testing.T) {
	t.Run("one byte", func(t *testing.T) {
		input := protocolClientFrame(8, true, []byte{0x03}, true)
		if _, _, err := readProtocolMessage(t, input, websocketProtocolLimits{frameBytes: 16, messageBytes: 32}); err == nil {
			t.Fatal("one-byte close payload was accepted")
		}
	})
	t.Run("invalid UTF-8 reason", func(t *testing.T) {
		input := protocolClientFrame(8, true, []byte{0x03, 0xe8, 0xff}, true)
		if _, _, err := readProtocolMessage(t, input, websocketProtocolLimits{frameBytes: 16, messageBytes: 32}); err == nil {
			t.Fatal("invalid close reason was accepted")
		}
	})
}

func TestReadWebSocketClientMessageNormalizesDisallowedCloseCodeLikeTungstenite(t *testing.T) {
	input := protocolClientFrame(8, true, []byte{0x03, 0xed}, true) // 1005: reserved status code.
	var output bytes.Buffer
	writer := &websocketFrameWriter{writer: bufio.NewWriter(&output)}
	payload, kind, err := readWebSocketClientMessageWithLimits(
		bufio.NewReader(bytes.NewReader(input)),
		writer,
		websocketProtocolLimits{frameBytes: 32, messageBytes: 32},
	)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil || kind != websocketInputClose {
		t.Fatalf("close result = kind %d payload %v", kind, payload)
	}
	frame, err := websocketframe.ReadHeader(&output)
	if err != nil {
		t.Fatal(err)
	}
	closePayload, err := frame.ReadPayload(&output, 125)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Opcode != 8 || frame.Masked() || binary.BigEndian.Uint16(closePayload[:2]) != 1002 ||
		string(closePayload[2:]) != "Protocol violation" {
		t.Fatalf("normalized close frame = opcode %d masked=%t payload=%v", frame.Opcode, frame.Masked(), closePayload)
	}
}

func TestReadWebSocketClientMessageEchoesAllowedCloseAndRespondsToPing(t *testing.T) {
	t.Run("close", func(t *testing.T) {
		closePayload := append([]byte{0x03, 0xe8}, "bye"...)
		input := protocolClientFrame(8, true, closePayload, true)
		var output bytes.Buffer
		writer := &websocketFrameWriter{writer: bufio.NewWriter(&output)}
		_, kind, err := readWebSocketClientMessageWithLimits(
			bufio.NewReader(bytes.NewReader(input)),
			writer,
			websocketProtocolLimits{frameBytes: 32, messageBytes: 32},
		)
		if err != nil {
			t.Fatal(err)
		}
		if kind != websocketInputClose {
			t.Fatalf("close kind = %d", kind)
		}
		frame, err := websocketframe.ReadHeader(&output)
		if err != nil {
			t.Fatal(err)
		}
		got, err := frame.ReadPayload(&output, 125)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, closePayload) {
			t.Fatalf("echoed close payload = %v, want %v", got, closePayload)
		}
	})
	t.Run("ping then text", func(t *testing.T) {
		input := append(protocolClientFrame(9, true, []byte("ping"), true), protocolClientFrame(1, true, []byte("hello"), true)...)
		var output bytes.Buffer
		writer := &websocketFrameWriter{writer: bufio.NewWriter(&output)}
		payload, kind, err := readWebSocketClientMessageWithLimits(
			bufio.NewReader(bytes.NewReader(input)),
			writer,
			websocketProtocolLimits{frameBytes: 32, messageBytes: 32},
		)
		if err != nil {
			t.Fatal(err)
		}
		if kind != websocketInputText || string(payload) != "hello" {
			t.Fatalf("text after ping = kind %d payload %q", kind, payload)
		}
		frame, err := websocketframe.ReadHeader(&output)
		if err != nil {
			t.Fatal(err)
		}
		pong, err := frame.ReadPayload(&output, 125)
		if err != nil {
			t.Fatal(err)
		}
		if frame.Opcode != 10 || string(pong) != "ping" {
			t.Fatalf("pong = opcode %d payload %q", frame.Opcode, pong)
		}
	})
}

func TestReadWebSocketClientMessageReturnsBinaryWithoutRoutingPayload(t *testing.T) {
	input := protocolClientFrame(2, true, []byte{1, 2, 3}, true)
	payload, kind, err := readProtocolMessage(t, input, websocketProtocolLimits{frameBytes: 8, messageBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil || kind != websocketInputBinary {
		t.Fatalf("binary result = kind %d payload %v", kind, payload)
	}
}

func TestForwardWebSocketClientFrameValidatesDirectionAndClosesBothPeers(t *testing.T) {
	var upstream, downstream bytes.Buffer
	state := websocketClientForwardState{}
	input := bytes.NewReader(protocolClientFrame(8, true, []byte{0x03, 0xe8}, true))
	frame, err := websocketframe.ReadHeader(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := forwardWebSocketClientFrame(
		input, &upstream, &websocketFrameWriter{writer: bufio.NewWriter(&downstream)}, frame, &state,
	); err != nil {
		t.Fatal(err)
	}
	if !state.closed {
		t.Fatal("close frame did not end the client stream")
	}
	for _, check := range []struct {
		frame  []byte
		masked bool
	}{{upstream.Bytes(), true}, {downstream.Bytes(), false}} {
		parsed, err := websocketframe.ReadHeader(bytes.NewReader(check.frame))
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Opcode != 8 || parsed.Masked() != check.masked {
			t.Fatalf("close frame = opcode %d masked=%t", parsed.Opcode, parsed.Masked())
		}
	}
}

func TestForwardWebSocketClientFrameRejectsUnmasked(t *testing.T) {
	input := bytes.NewReader(protocolClientFrame(1, true, []byte("text"), false))
	frame, err := websocketframe.ReadHeader(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := forwardWebSocketClientFrame(input, io.Discard, &websocketFrameWriter{writer: bufio.NewWriter(io.Discard)}, frame, &websocketClientForwardState{}); err == nil {
		t.Fatal("accepted an unmasked client frame")
	}
}

func TestCopyWebSocketFramesAnswersUpstreamPingAndEchoesClose(t *testing.T) {
	input := append(protocolClientFrame(9, true, []byte("ping"), false), protocolClientFrame(8, true, []byte{0x03, 0xe8}, false)...)
	connection := &protocolFrameConn{Reader: bytes.NewReader(input)}
	var downstream bytes.Buffer
	if err := copyWebSocketFrames(connection, &websocketFrameWriter{writer: bufio.NewWriter(&downstream)}); err != nil {
		t.Fatal(err)
	}
	upstreamReader := bytes.NewReader(connection.writes.Bytes())
	upstream, err := websocketframe.ReadHeader(upstreamReader)
	if err != nil {
		t.Fatal(err)
	}
	pong, err := upstream.ReadPayload(upstreamReader, 125)
	if err != nil {
		t.Fatal(err)
	}
	upstream.Unmask(pong)
	if upstream.Opcode != 10 || !upstream.Masked() || string(pong) != "ping" {
		t.Fatalf("upstream pong = opcode %d masked=%t payload=%q", upstream.Opcode, upstream.Masked(), pong)
	}
	client, err := websocketframe.ReadHeader(&downstream)
	if err != nil {
		t.Fatal(err)
	}
	clientClose, err := client.ReadPayload(&downstream, 125)
	if err != nil {
		t.Fatal(err)
	}
	if client.Opcode != 8 || client.Masked() || !bytes.Equal(clientClose, []byte{0x03, 0xe8}) {
		t.Fatalf("downstream close = opcode %d masked=%t payload=%v", client.Opcode, client.Masked(), clientClose)
	}
	upstreamClose, err := websocketframe.ReadHeader(upstreamReader)
	if err != nil {
		t.Fatalf("upstream close frame: %v", err)
	}
	closePayload, err := upstreamClose.ReadPayload(upstreamReader, 125)
	if err != nil {
		t.Fatal(err)
	}
	upstreamClose.Unmask(closePayload)
	if upstreamClose.Opcode != 8 || !upstreamClose.Masked() || !bytes.Equal(closePayload, []byte{0x03, 0xe8}) {
		t.Fatalf("upstream close = opcode %d masked=%t payload=%v", upstreamClose.Opcode, upstreamClose.Masked(), closePayload)
	}
	if err := copyWebSocketFrames(bytes.NewReader(protocolClientFrame(1, true, []byte("text"), true)), &websocketFrameWriter{writer: bufio.NewWriter(io.Discard)}); err == nil {
		t.Fatal("accepted a masked upstream data frame")
	}
}

type protocolFrameConn struct {
	*bytes.Reader
	writes bytes.Buffer
}

func (connection *protocolFrameConn) Write(payload []byte) (int, error) {
	return connection.writes.Write(payload)
}

func (*protocolFrameConn) Close() error { return nil }

func TestWebSocketCloseCodePolicyMatchesTungstenite030(t *testing.T) {
	for _, test := range []struct {
		code uint16
		want bool
	}{
		{1000, true}, {1003, true}, {1004, false}, {1005, false}, {1006, false},
		{1007, true}, {1013, true}, {1014, false}, {1015, false},
		{2999, false}, {3000, true}, {4999, true}, {5000, false},
	} {
		if got := websocketframe.WebSocketCloseCodeAllowed(test.code); got != test.want {
			t.Fatalf("close code %d allowed=%t want=%t", test.code, got, test.want)
		}
	}
}

func readProtocolMessage(
	t *testing.T,
	input []byte,
	limits websocketProtocolLimits,
) ([]byte, websocketInputKind, error) {
	t.Helper()
	var output bytes.Buffer
	return readWebSocketClientMessageWithLimits(
		bufio.NewReader(bytes.NewReader(input)),
		&websocketFrameWriter{writer: bufio.NewWriter(&output)},
		limits,
	)
}

func protocolClientFrame(opcode byte, final bool, payload []byte, masked bool) []byte {
	first := opcode
	if final {
		first |= 0x80
	}
	return protocolClientFrameWithFirstByte(first, payload, masked)
}

func protocolClientFrameWithFirstByte(first byte, payload []byte, masked bool) []byte {
	var frame bytes.Buffer
	frame.WriteByte(first)
	length := len(payload)
	second := byte(0)
	switch {
	case length < 126:
		second = byte(length)
		frame.WriteByte(second | protocolMaskBit(masked))
	case length <= 0xffff:
		frame.WriteByte(126 | protocolMaskBit(masked))
		var size [2]byte
		binary.BigEndian.PutUint16(size[:], uint16(length))
		frame.Write(size[:])
	default:
		frame.WriteByte(127 | protocolMaskBit(masked))
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(length))
		frame.Write(size[:])
	}
	if !masked {
		frame.Write(payload)
		return frame.Bytes()
	}
	mask := [4]byte{1, 2, 3, 4}
	frame.Write(mask[:])
	for index, value := range payload {
		frame.WriteByte(value ^ mask[index%len(mask)])
	}
	return frame.Bytes()
}

func protocolMaskBit(masked bool) byte {
	if masked {
		return 0x80
	}
	return 0
}
