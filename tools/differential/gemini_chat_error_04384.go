package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func geminiChatErrorWireValid04384(wire string) bool {
	if !validFixtureStreamErrorPresence04384("deepseek-sse-error-presence", wire) {
		return false
	}
	var event struct {
		CreatedAt int64 `json:"created_at"`
		Response  struct{ ID string }
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(wire, "event: response.failed\r\ndata: "), "\r\n\r\n")
	if json.Unmarshal([]byte(payload), &event) != nil {
		return false
	}
	want := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":%d,\"response\":{\"error\":{\"code\":\"provider_stream_error\",\"message\":\"Provider stream returned an embedded error\"},\"id\":%q},\"sequence_number\":0,\"type\":\"response.failed\"}\r\n\r\n", event.CreatedAt, event.Response.ID)
	return wire == want
}

func geminiChatErrorSentinel04384() bool {
	good := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":%d,\"response\":{\"error\":{\"code\":\"provider_stream_error\",\"message\":\"Provider stream returned an embedded error\"},\"id\":\"resp_deepseek_01900000-0000-7000-8000-000000000001\"},\"sequence_number\":0,\"type\":\"response.failed\"}\r\n\r\n", time.Now().Unix())
	if !geminiChatErrorWireValid04384(good) {
		return false
	}
	for _, changed := range []string{
		strings.Replace(good, "provider_stream_error", "must-not-borrow", 1),
		strings.Replace(good, "Provider stream returned an embedded error", "must-not-borrow", 1),
		strings.Replace(good, `"sequence_number":0`, `"sequence_number":1`, 1),
		strings.Replace(good, "event: response.failed", "event: response.completed", 1),
		strings.ReplaceAll(good, "\r\n", "\n"), good + good,
	} {
		if changed == good || geminiChatErrorWireValid04384(changed) {
			return false
		}
	}
	return true
}
