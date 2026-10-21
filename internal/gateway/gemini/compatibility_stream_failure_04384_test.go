package gemini

import (
	"bytes"
	"testing"
)

func TestProdex04384GeminiAPIKeyFailurePreservesCreationEpoch(t *testing.T) {
	state := &geminiChatCompatibleStream043711{id: "resp-owned", created: 42}
	var output bytes.Buffer
	state.failure(&output, "unexpected end of stream")
	want := "event: response.failed\r\ndata: " + `{"created_at":42,"response":{"error":{"code":"provider_stream_error","message":"unexpected end of stream"},"id":"resp-owned"},"sequence_number":0,"type":"response.failed"}` + "\r\n\r\n"
	if output.String() != want || !state.done {
		t.Fatalf("failure lost canonical creation epoch/shape: %q want=%q", output.String(), want)
	}
	state.failure(&output, "duplicate")
	if err := state.complete(&output); err != nil || output.String() != want {
		t.Errorf("failure was followed by another terminal event: err=%v output=%q", err, output.String())
	}
}
