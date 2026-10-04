package openai

import (
	"bytes"
	"io"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

type websocketEventFixtureConn struct {
	reader *bytes.Reader
	writes bytes.Buffer
}

func (conn *websocketEventFixtureConn) Read(buffer []byte) (int, error) {
	return conn.reader.Read(buffer)
}

func (conn *websocketEventFixtureConn) Write(buffer []byte) (int, error) {
	return conn.writes.Write(buffer)
}

func (*websocketEventFixtureConn) Close() error { return nil }

func TestReadWebSocketEventClassifiesInvalidPreviousResponseLikeProdex04354(t *testing.T) {
	payload := []byte("{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Invalid `previous_response_id`.\"}}")
	event := readWebSocketEventFixture(t, payload)
	if !event.invalidPreviousResponseID ||
		event.retryCode != "previous_response_not_found" ||
		!event.terminal {
		t.Fatalf("invalid previous event = %#v", event)
	}
}

func TestReadWebSocketEventPreservesExplicitPreviousResponseNotFoundPrecedence(t *testing.T) {
	payload := []byte("{\"type\":\"response.failed\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"code\":\"previous_response_not_found\",\"message\":\"Invalid `previous_response_id`.\"}}")
	event := readWebSocketEventFixture(t, payload)
	if event.invalidPreviousResponseID ||
		event.retryCode != "previous_response_not_found" ||
		!event.terminal {
		t.Fatalf("explicit not-found event = %#v", event)
	}
}

func TestReadWebSocketEventDoesNotRetryUnrelatedInvalidRequest(t *testing.T) {
	payload := []byte("{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"invalid model\"}}")
	event := readWebSocketEventFixture(t, payload)
	if event.invalidPreviousResponseID || event.retryCode != "" || !event.terminal {
		t.Fatalf("unrelated invalid event = %#v", event)
	}
}

func readWebSocketEventFixture(t *testing.T, payload []byte) websocketEvent {
	t.Helper()
	var encoded bytes.Buffer
	if err := websocketframe.WriteFrame(&encoded, 1, payload, false); err != nil {
		t.Fatal(err)
	}
	conn := &websocketEventFixtureConn{reader: bytes.NewReader(encoded.Bytes())}
	event, err := readWebSocketEvent(conn)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	return event
}
