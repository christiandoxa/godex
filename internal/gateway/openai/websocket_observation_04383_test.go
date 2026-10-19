package openai

import (
	"bytes"
	"io"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04383WebSocketObservationDoesNotWritePerToken(t *testing.T) {
	conn := &inMemoryServerFrames04380{}
	for range 200 {
		if err := websocketframe.WriteFrame(conn, 1, []byte(`{"type":"response.output_text.delta","response_id":"resp-known","delta":"x"}`), false); err != nil {
			t.Fatal(err)
		}
	}
	const terminal = `{"type":"response.completed","response":{"id":"resp-known","usage":{"output_tokens":200}}}`
	if err := websocketframe.WriteFrame(conn, 1, []byte(terminal), false); err != nil {
		t.Fatal(err)
	}
	body := newWebSocketResponseBody(conn, nil, "", &websocketMessageState{}, nil)
	body.observation = &proxymodel.WebSocketEvent{}
	body.responseID = "resp-known"
	defer body.Close()
	observations := 0
	buffer := make([]byte, 13)
	for {
		count, err := body.Read(buffer)
		if body.observation.Sequence > uint64(observations) {
			observations++
			if count == 0 || string(body.observation.Payload) != terminal {
				t.Fatalf("observation arrived after bytes or changed payload: count=%d payload=%s", count, body.observation.Payload)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if observations != 1 {
		t.Fatalf("repeated token frames triggered persistence: %d observations", observations)
	}
}

func TestProdex04383TruncatedWebSocketMessageCannotCreateObservation(t *testing.T) {
	conn := &inMemoryServerFrames04380{}
	conn.Write([]byte{0x81, 40, '{', '"'})
	body := newWebSocketResponseBody(conn, nil, "", nil, nil)
	body.observation = &proxymodel.WebSocketEvent{}
	defer body.Close()
	_, err := io.Copy(&bytes.Buffer{}, body)
	if err == nil || body.observation.Sequence != 0 {
		t.Fatalf("partial payload claimed a complete observation: err=%v event=%+v", err, body.observation)
	}
}
