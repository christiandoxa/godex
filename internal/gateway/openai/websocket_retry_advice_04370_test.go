package openai

import (
	"bytes"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04370WebSocketPrecommitKeepsBoundedStructuredHeaderEvidence(t *testing.T) {
	source := []byte("{\"type\":\"error\",\"status\":429,\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"retry in 1s\",\"headers\":{\"Retry-After\":\"5\"}}}")
	event := websocketEvent{text: true, kind: "error", payload: source, retryCode: "rate_limit_exceeded"}
	result := websocketPrecommitFailure(nil, "", "", false, event, false, 0)
	if result.PrecommitFailure == nil || !bytes.Equal(result.PrecommitFailure.RetryAdviceJSON, source) {
		t.Fatal("precommit WebSocket failed to retain original structured retry metadata")
	}
	source[0] = 'x'
	if !bytes.Equal(result.PrecommitFailure.RetryAdviceJSON,
		[]byte("{\"type\":\"error\",\"status\":429,\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"retry in 1s\",\"headers\":{\"Retry-After\":\"5\"}}}")) {
		t.Fatal("websocket retry metadata aliases mutable input buffer")
	}
	big := bytes.Repeat([]byte("x"), 64<<10+1)
	result = websocketPrecommitFailure(nil, "", "", false, websocketEvent{text: true, payload: big, retryCode: "rate_limit_exceeded"}, false, 0)
	if result.PrecommitFailure == nil || len(result.PrecommitFailure.RetryAdviceJSON) != 0 {
		t.Fatal("oversized WebSocket event was preserved in unbounded routing metadata")
	}
	result = websocketPrecommitFailure(nil, "", "", false, websocketEvent{text: false, payload: []byte("x"), retryCode: "rate_limit_exceeded"}, false, 0)
	if len(result.PrecommitFailure.RetryAdviceJSON) != 0 {
		t.Fatal("binary WebSocket message interpreted as JSON retry metadata")
	}
}

var _ *proxymodel.PrecommitFailure // Preserve the gateway/model contract.
