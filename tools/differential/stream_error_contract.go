package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var deepSeekResponseIDPattern = regexp.MustCompile(
	"^resp_deepseek_[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$",
)

// A DeepSeek provider error is already inside the translated SSE writer in
// Prodex 0.437.0. It must be delivered as one response.failed frame, not
// silently retried on another key. Only source-generated timestamp and
// UUIDv7 are normalized after strict validation of every other field.
func canonicalFixtureFailedSSE(body string) (string, error) {
	if !strings.HasSuffix(body, "\r\n\r\n") {
		return "", errors.New("failed SSE event missing terminator")
	}
	block := strings.TrimSuffix(body, "\r\n\r\n")
	lines := strings.Split(block, "\r\n")
	if len(lines) != 2 || lines[0] != "event: response.failed" ||
		!strings.HasPrefix(lines[1], "data: ") {
		return "", errors.New("unexpected failed SSE frame count or type")
	}
	var event map[string]any
	if json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &event) != nil {
		return "", errors.New("invalid failed SSE JSON")
	}
	created, ok := event["created_at"].(float64)
	if !ok || created <= 0 || created > float64(1<<53) {
		return "", errors.New("invalid failed SSE creation time")
	}
	clock := time.Unix(int64(created), 0)
	if delta := time.Since(clock); delta > 5*time.Minute || delta < -5*time.Minute {
		return "", errors.New("out-of-window stream failure timestamp")
	}
	response, ok := event["response"].(map[string]any)
	if !ok {
		return "", errors.New("missing failed response")
	}
	id, ok := response["id"].(string)
	if !ok || !deepSeekResponseIDPattern.MatchString(id) {
		return "", errors.New("invalid generated response UUIDv7")
	}
	event["created_at"] = float64(0)
	response["id"] = "<opaque-response-id>"
	expected := map[string]any{
		"type":            "response.failed",
		"sequence_number": float64(0),
		"created_at":      float64(0),
		"response": map[string]any{
			"id": "<opaque-response-id>",
			"error": map[string]any{
				"code":    "rate_limit_exceeded",
				"message": "Please try again in 1s.",
			},
		},
	}
	if !reflect.DeepEqual(event, expected) {
		return "", errors.New("failed stream error semantics changed")
	}
	encoded, err := json.Marshal(event)
	return string(encoded), err
}

func validFixtureFailedSSE(body string) bool {
	_, err := canonicalFixtureFailedSSE(body)
	return err == nil
}

func equivalentFixtureFailedSSE(reference, candidate string) bool {
	left, errLeft := canonicalFixtureFailedSSE(reference)
	right, errRight := canonicalFixtureFailedSSE(candidate)
	return errLeft == nil && errRight == nil && left == right
}
