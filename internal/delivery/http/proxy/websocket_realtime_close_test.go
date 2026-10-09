package proxy

import (
	"bufio"
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

func TestRealtimeUpstreamCloseIsAcknowledgedAndEndsCleanly(t *testing.T) {
	local, localPeer := net.Pipe()
	upstream, upstreamPeer := net.Pipe()
	defer localPeer.Close()
	defer upstreamPeer.Close()
	_ = upstreamPeer.SetDeadline(time.Now().Add(time.Second))

	var downstream bytes.Buffer
	toClient := &websocketFrameWriter{writer: bufio.NewWriter(&downstream)}
	stop := func() {
		_ = local.Close()
		_ = upstream.Close()
	}
	result := make(chan error, 1)
	go func() {
		result <- runRealtimeWebSocketDuplex(bufio.NewReader(local), toClient, upstream, stop)
	}()

	closePayload := []byte{0x03, 0xe8}
	if err := websocketframe.WriteFrame(upstreamPeer, 8, closePayload, false); err != nil {
		t.Fatal(err)
	}
	ack, err := websocketframe.ReadHeader(upstreamPeer)
	if err != nil {
		t.Fatal(err)
	}
	ackPayload, err := ack.ReadPayload(upstreamPeer, 125)
	if err != nil {
		t.Fatal(err)
	}
	ack.Unmask(ackPayload)
	if ack.Opcode != 8 || !ack.Masked() || !bytes.Equal(ackPayload, closePayload) {
		t.Fatalf("upstream close acknowledgement = opcode %d masked=%t payload=%v", ack.Opcode, ack.Masked(), ackPayload)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("clean upstream close returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("realtime session did not stop after upstream close")
	}

	frame, err := websocketframe.ReadHeader(&downstream)
	if err != nil {
		t.Fatal(err)
	}
	forwarded, err := frame.ReadPayload(&downstream, 125)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Opcode != 8 || frame.Masked() || !bytes.Equal(forwarded, closePayload) {
		t.Fatalf("downstream close = opcode %d masked=%t payload=%v", frame.Opcode, frame.Masked(), forwarded)
	}
}
