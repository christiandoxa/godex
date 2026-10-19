package proxy

import (
	"errors"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04383WebSocketPersistenceFailureStopsAssociatedBytes(t *testing.T) {
	want := errors.New("synthetic persistence failure")
	reader := &websocketObservedReader04383{
		Reader: strings.NewReader("final payload"), event: &proxymodel.WebSocketEvent{Sequence: 1, Payload: []byte(`{"response":{"id":"resp-late"}}`)},
		observe: func([]byte) error { return want },
	}
	count, err := reader.Read(make([]byte, 32))
	if count != 0 || !errors.Is(err, want) {
		t.Fatalf("persistence failure forwarded associated bytes: count=%d err=%v", count, err)
	}
}
