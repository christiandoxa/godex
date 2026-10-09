package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

var fixtureStreamEventTypes = []string{
	"response.created",
	"response.output_item.added",
	"response.output_text.delta",
	"response.output_item.done",
	"response.completed",
}

const syntheticResponseID = "chatcmpl-differential-stream"

// canonicalFixtureSSE accepts only the exact five-event DeepSeek fixture wire
// contract and normalizes one opaque output item ID. Everything else must
// match the tagged Prodex reference, including sequence, text, and metadata.
func canonicalFixtureSSE(body string) (string, error) {
	if !strings.HasSuffix(body, "\r\n\r\n") {
		return "", errors.New("SSE terminator missing")
	}
	blocks := strings.Split(strings.TrimSuffix(body, "\r\n\r\n"), "\r\n\r\n")
	if len(blocks) != len(fixtureStreamEventTypes) {
		return "", fmt.Errorf("SSE event count %d, want %d", len(blocks), len(fixtureStreamEventTypes))
	}
	var opaqueID string
	events := make([]any, 0, len(blocks))
	for index, block := range blocks {
		lines := strings.Split(block, "\r\n")
		if len(lines) != 2 || lines[0] != "event: "+fixtureStreamEventTypes[index] ||
			!strings.HasPrefix(lines[1], "data: ") {
			return "", fmt.Errorf("unexpected SSE frame at %d", index)
		}
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &event) != nil ||
			event["type"] != fixtureStreamEventTypes[index] ||
			event["sequence_number"] != float64(index) {
			return "", fmt.Errorf("SSE metadata invalid at %d", index)
		}
		if err := validateFixtureStreamEvent(index, event, &opaqueID); err != nil {
			return "", fmt.Errorf("event %d: %w", index, err)
		}
		events = append(events, event)
	}
	// Compare every key and nested value to the canonical stream, not just
	// selected text fields. An extra event/field cannot become a false PASS.
	var expected []any
	const exact = `[
      {"type":"response.created","sequence_number":0,"created_at":1,"response":{"id":"chatcmpl-differential-stream"}},
      {"type":"response.output_item.added","sequence_number":1,"response_id":"chatcmpl-differential-stream","item":{"id":"<opaque-message-id>","type":"message","role":"assistant","content":[]}},
      {"type":"response.output_text.delta","sequence_number":2,"created_at":1,"response_id":"chatcmpl-differential-stream","delta":"synthetic-ok"},
      {"type":"response.output_item.done","sequence_number":3,"response_id":"chatcmpl-differential-stream","item":{"id":"<opaque-message-id>","type":"message","role":"assistant","content":[{"type":"output_text","text":"synthetic-ok"}]}},
      {"type":"response.completed","sequence_number":4,"created_at":1,"response":{"id":"chatcmpl-differential-stream","metadata":{"deepseek":{"finish_reason":"stop"}},"model":"deepseek-v4-pro","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"synthetic-ok"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}
    ]`
	if err := json.Unmarshal([]byte(exact), &expected); err != nil {
		return "", err
	}
	if !reflect.DeepEqual(events, expected) {
		return "", errors.New("stream event fields differ from canonical fixture")
	}
	normalized, err := json.Marshal(events)
	return string(normalized), err
}

func validFixtureSSE(body string) bool {
	_, err := canonicalFixtureSSE(body)
	return err == nil
}

func equivalentFixtureSSE(left, right string) bool {
	a, ea := canonicalFixtureSSE(left)
	b, eb := canonicalFixtureSSE(right)
	return ea == nil && eb == nil && a == b
}

func validateFixtureStreamEvent(index int, event map[string]any, opaqueID *string) error {
	switch index {
	case 0:
		if event["created_at"] != float64(1) ||
			!reflect.DeepEqual(event["response"], map[string]any{"id": syntheticResponseID}) {
			return errors.New("created response identity or clock differs")
		}
	case 1, 3:
		if event["response_id"] != syntheticResponseID {
			return errors.New("message response identity differs")
		}
		item, ok := event["item"].(map[string]any)
		if !ok || item["type"] != "message" || item["role"] != "assistant" {
			return errors.New("message item kind differs")
		}
		id, ok := item["id"].(string)
		if !ok || !strings.HasPrefix(id, "msg_deepseek_") {
			return errors.New("message ID format differs")
		}
		number := strings.TrimPrefix(id, "msg_deepseek_")
		if _, err := strconv.ParseUint(number, 10, 64); err != nil {
			return errors.New("message ID is not unsigned decimal")
		}
		if *opaqueID == "" {
			*opaqueID = id
		} else if id != *opaqueID {
			return errors.New("message ID changed between added and done")
		}
		item["id"] = "<opaque-message-id>"
		expectedContent := any([]any{})
		if index == 3 {
			expectedContent = []any{map[string]any{"text": "synthetic-ok", "type": "output_text"}}
		}
		if !reflect.DeepEqual(item["content"], expectedContent) {
			return errors.New("message content differs")
		}
	case 2:
		if event["created_at"] != float64(1) ||
			event["response_id"] != syntheticResponseID ||
			event["delta"] != "synthetic-ok" {
			return errors.New("stream text delta differs")
		}
	case 4:
		if event["created_at"] != float64(1) {
			return errors.New("completion timestamp differs")
		}
		var expected any
		const completed = `{"id":"chatcmpl-differential-stream","metadata":{"deepseek":{"finish_reason":"stop"}},"model":"deepseek-v4-pro","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
		if json.Unmarshal([]byte(completed), &expected) != nil ||
			!reflect.DeepEqual(event["response"], expected) {
			return errors.New("completed response data differs")
		}
	}
	return nil
}

// This scenario uses a pinned future epoch, not a relative server clock.
// All seven client-visible streaming headers must match the provider's
// Codex rate-limit projection, with no leaked raw upstream fields.
func validFixtureStreamingQuotaHeaders(source http.Header) bool {
	expected := http.Header{
		"Content-Type":                             []string{"text/event-stream; charset=utf-8"},
		"X-Deepseek-Requests-Limit-Name":           []string{"DeepSeek requests"},
		"X-Deepseek-Requests-Primary-Used-Percent": []string{"25"},
		"X-Deepseek-Requests-Primary-Reset-At":     []string{"1893456000"},
		"X-Deepseek-Tokens-Limit-Name":             []string{"DeepSeek tokens"},
		"X-Deepseek-Tokens-Primary-Used-Percent":   []string{"100"},
		"X-Deepseek-Tokens-Primary-Reset-At":       []string{"1893456000"},
	}
	return reflect.DeepEqual(canonicalHeaders(source), canonicalHeaders(expected))
}
