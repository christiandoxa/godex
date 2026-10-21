package gemini

import (
	"bytes"
	"testing"
)

func TestProdex04384GeminiAPIKeyFailurePreservesCreationEpoch(t *testing.T) {
	state := &geminiChatCompatibleStream043711{id: "resp-owned", created: 42}
	var output bytes.Buffer
	if err := state.failure(&output, "provider_stream_error", "unexpected end of stream"); err != nil {
		t.Fatal(err)
	}
	want := "event: response.failed\r\ndata: " + `{"created_at":42,"response":{"error":{"code":"provider_stream_error","message":"unexpected end of stream"},"id":"resp-owned"},"sequence_number":0,"type":"response.failed"}` + "\r\n\r\n"
	if output.String() != want || !state.done {
		t.Fatalf("failure lost canonical creation epoch/shape: %q want=%q", output.String(), want)
	}
	if err := state.failure(&output, "provider_stream_error", "duplicate"); err != nil {
		t.Fatal(err)
	}
	if err := state.complete(&output); err != nil || output.String() != want {
		t.Errorf("failure was followed by another terminal event: err=%v output=%q", err, output.String())
	}
}
