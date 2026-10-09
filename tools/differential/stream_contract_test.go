package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func fixtureStreamWire(id string) string {
	const placeholder = "__ITEM_ID__"
	events := []string{
		`{"created_at":1,"response":{"id":"chatcmpl-differential-stream"},"sequence_number":0,"type":"response.created"}`,
		`{"item":{"content":[],"id":"__ITEM_ID__","role":"assistant","type":"message"},"response_id":"chatcmpl-differential-stream","sequence_number":1,"type":"response.output_item.added"}`,
		`{"created_at":1,"delta":"synthetic-ok","response_id":"chatcmpl-differential-stream","sequence_number":2,"type":"response.output_text.delta"}`,
		`{"item":{"content":[{"text":"synthetic-ok","type":"output_text"}],"id":"__ITEM_ID__","role":"assistant","type":"message"},"response_id":"chatcmpl-differential-stream","sequence_number":3,"type":"response.output_item.done"}`,
		`{"created_at":1,"response":{"id":"chatcmpl-differential-stream","metadata":{"deepseek":{"finish_reason":"stop"}},"model":"deepseek-v4-pro","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}},"sequence_number":4,"type":"response.completed"}`,
	}
	var result strings.Builder
	for i, event := range events {
		event = strings.ReplaceAll(event, placeholder, id)
		fmt.Fprintf(&result, "event: %s\r\ndata: %s\r\n\r\n", fixtureStreamEventTypes[i], event)
	}
	return result.String()
}

func TestFixtureSSEOnlyNormalizesConsistentOpaqueMessageID(t *testing.T) {
	baseline := fixtureStreamWire("msg_deepseek_123456789")
	candidate := fixtureStreamWire("msg_deepseek_7")
	if !validFixtureSSE(baseline) || !validFixtureSSE(candidate) ||
		!equivalentFixtureSSE(baseline, candidate) {
		t.Fatal("identical streaming event contract with different opaque IDs rejected")
	}
	if validFixtureStreamingRequest(syntheticFixtureRequest) {
		t.Fatal("buffered input accepted as stream")
	}
	const streamingRequest = `{"model":"deepseek-v4-pro","stream":true,"messages":[{"role":"user","content":"same request"}]}`
	if !validFixtureStreamingRequest(streamingRequest) || validFixtureRequest(streamingRequest) {
		t.Fatal("streaming request identity confused with buffered fixture")
	}
}

func TestFixtureSSENegativeControlsRejectMutatedObservableBehavior(t *testing.T) {
	good := fixtureStreamWire("msg_deepseek_42")
	if _, err := canonicalFixtureSSE(good); err != nil {
		t.Fatalf("canonical fixture rejected: %v", err)
	}
	fixtures := []struct {
		name   string
		mutate func(string) string
	}{
		{"text", func(s string) string { return strings.Replace(s, "synthetic-ok", "wrong-output", 1) }},
		{"usage", func(s string) string { return strings.Replace(s, `"total_tokens":3`, `"total_tokens":99`, 1) }},
		{"sequence", func(s string) string { return strings.Replace(s, `"sequence_number":2`, `"sequence_number":9`, 1) }},
		{"event_name", func(s string) string {
			return strings.Replace(s, "event: response.output_item.done", "event: response.output_text.done", 1)
		}},
		{"response_id", func(s string) string { return strings.Replace(s, "chatcmpl-differential-stream", "wrong-response", 1) }},
		{"inconsistent_id", func(s string) string {
			return strings.Replace(s, `"id":"msg_deepseek_42"`, `"id":"msg_deepseek_43"`, 1)
		}},
		{"invalid_id", func(s string) string { return strings.ReplaceAll(s, "msg_deepseek_42", "msg_deepseek_bogus") }},
		{"extra_field", func(s string) string { return strings.Replace(s, `"created_at":1`, `"extra":"bad","created_at":1`, 1) }},
		{"extra_event", func(s string) string { return s + "event: response.completed\r\ndata: {}\r\n\r\n" }},
		{"missing_terminator", func(s string) string { return strings.TrimSuffix(s, "\r\n\r\n") }},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			malformed := fixture.mutate(good)
			if validFixtureSSE(malformed) || equivalentFixtureSSE(good, malformed) {
				t.Fatalf("mutated stream %s passed parity oracle", fixture.name)
			}
		})
	}
}

func TestFixtureSSERejectsWrongJSONAndNoHiddenNormalizer(t *testing.T) {
	invalid := fixtureStreamWire("msg_deepseek_42")
	invalid = strings.Replace(invalid, `"delta":"synthetic-ok"`, `"delta":null`, 1)
	if validFixtureSSE(invalid) {
		t.Fatal("null delta accepted")
	}
	const streamRequest = `{"model":"deepseek-v4-pro","stream":true,"messages":[{"role":"user","content":"same request"}]}`
	var request map[string]any
	if err := json.Unmarshal([]byte(streamRequest), &request); err != nil {
		t.Fatal(err)
	}
	request["model"] = "wrong"
	content, _ := json.Marshal(request)
	if validFixtureStreamingRequest(string(content)) {
		t.Fatal("incorrect upstream model accepted")
	}
}
