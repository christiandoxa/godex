package main

import (
	"strings"
	"testing"
)

func TestProdex043711GeminiNativeSSEStrictOracleRejectsFalseParity(t *testing.T) {
	good := "event: response.created\r\ndata: " + `{"created_at":111,"response":{"id":"resp-fixture"},"sequence_number":0,"type":"response.created"}` +
		"\r\n\r\nevent: response.completed\r\ndata: " +
		`{"created_at":111,"response":{"id":"resp-fixture","output":[]},"sequence_number":1,"type":"response.completed"}` + "\r\n\r\n"
	if _, err := geminiNativeSSEProjection043711(good); err != nil {
		t.Fatal(err)
	}
	similar := strings.ReplaceAll(strings.ReplaceAll(good, "resp-fixture", "resp-other"), "111", "999")
	if !geminiNativeSSEEqual043711(good, similar) {
		t.Fatal("arbitrary generated ID and timestamp were falsely counted as semantic difference")
	}
	for _, tc := range []struct{ name, changed string }{
		{"client delta leaked", strings.Replace(good, "response.completed", "response.output_text.delta", 1)},
		{"wrong sequence", strings.Replace(good, `"sequence_number":1`, `"sequence_number":2`, 1)},
		{"wrong second response identity", strings.Replace(good, `"resp-fixture","output"`, `"resp-other","output"`, 1)},
		{"fabricated final text", strings.Replace(good, `"output":[]`, `"output":[{"type":"message"}]`, 1)},
		{"non-Gemini field", strings.Replace(good, `"output":[]`, `"output":[],"unsupported":true`, 1)},
		{"missing terminal", strings.Split(good, "event: response.completed")[0]},
		{"duplicate completion", good + strings.Split(good, "event: response.completed")[1]},
		{"unframed JSON", strings.Replace(good, "event: response.created", "event: invalid", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := geminiNativeSSEProjection043711(tc.changed); err == nil {
				t.Fatalf("accepted noncanonical native Gemini SSE: %q", tc.changed)
			}
		})
	}
}

func TestProdex043711GeminiChatSSEItemAndUsageNegativeControls(t *testing.T) {
	if !geminiChatSSESentinel043711() {
		t.Fatal("Gemini Chat SSE oracle missed identity, sequence, usage, metadata or output mutations")
	}
}
