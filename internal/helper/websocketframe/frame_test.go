package websocketframe

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestWriteFrameAndReadHeaderRoundTrip(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		opcode byte
		length int
	}{
		{name: "short", opcode: 1, length: 5},
		{name: "extended 16-bit", opcode: 1, length: 126},
		{name: "extended 64-bit", opcode: 1, length: 65536},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte("x"), fixture.length)
			var encoded bytes.Buffer
			if err := WriteFrame(&encoded, fixture.opcode, payload, true); err != nil {
				t.Fatal(err)
			}
			frame, err := ReadHeader(&encoded)
			if err != nil {
				t.Fatal(err)
			}
			if frame.Opcode != fixture.opcode || !frame.Final || !frame.Masked() || frame.PayloadLength != uint64(len(payload)) {
				t.Fatalf("frame header = %#v", frame)
			}
			masked, err := frame.ReadPayload(&encoded, uint64(len(payload)))
			if err != nil {
				t.Fatal(err)
			}
			frame.Unmask(masked)
			if !bytes.Equal(masked, payload) {
				t.Fatal("decoded payload differs")
			}
		})
	}
}

func TestReadHeaderRejectsUnsupportedPayloadLength(t *testing.T) {
	var encoded [10]byte
	encoded[0], encoded[1] = 0x81, 127
	binary.BigEndian.PutUint64(encoded[2:], 1<<63)
	if _, err := ReadHeader(bytes.NewReader(encoded[:])); err == nil {
		t.Fatal("accepted a payload length with the reserved high bit set")
	}
}

func TestReadHeaderRejectsNonMinimalPayloadLength(t *testing.T) {
	short := []byte{0x81, 126, 0, 1, 'x'}
	if _, err := ReadHeader(bytes.NewReader(short)); err == nil {
		t.Fatal("accepted a 16-bit encoding for a one-byte payload")
	}
	long := make([]byte, 10)
	long[0], long[1] = 0x81, 127
	binary.BigEndian.PutUint64(long[2:], 1<<16-1)
	if _, err := ReadHeader(bytes.NewReader(long)); err == nil {
		t.Fatal("accepted a 64-bit encoding for a 16-bit payload")
	}
}

func TestNormalizeClosePayload(t *testing.T) {
	if _, err := NormalizeClosePayload([]byte{1}); err == nil {
		t.Fatal("accepted a one-byte close payload")
	}
	if _, err := NormalizeClosePayload([]byte{0x03, 0xe8, 0xff}); err == nil {
		t.Fatal("accepted an invalid UTF-8 close reason")
	}
	normalized, err := NormalizeClosePayload([]byte{0x03, 0xed})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(normalized, append([]byte{0x03, 0xea}, "Protocol violation"...)) {
		t.Fatalf("normalized close payload = %v", normalized)
	}
}

func TestReadHeaderDistinguishesCleanAndPartialEOF(t *testing.T) {
	if _, err := ReadHeader(bytes.NewReader(nil)); err != io.EOF {
		t.Fatalf("empty input error = %v", err)
	}
	if _, err := ReadHeader(bytes.NewReader([]byte{0x81})); err != io.ErrUnexpectedEOF {
		t.Fatalf("partial header error = %v", err)
	}
}
