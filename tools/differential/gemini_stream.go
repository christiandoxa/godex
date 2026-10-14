package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

// The pinned Prodex 0.437.11 Super API-key Gemini path treats a native
// GenerateContent SSE body as Chat Completions SSE. Its unsupported native
// chunks are invisible to the client; only created and completed frames
// survive, with no invented output. IDs and timestamps are normalized only
// after validating their internal consistency and event framing.
func geminiNativeSSEProjection043711(wire string) ([]map[string]any, error) {
	decoder := sse.NewDecoder(1 << 20)
	events := decoder.Feed([]byte(wire))
	events = append(events, decoder.Finish()...)
	if len(events) != 2 {
		return nil, fmt.Errorf("Gemini native SSE emitted %d events, want exactly two", len(events))
	}
	expected := []string{"response.created", "response.completed"}
	var responseID string
	var timestamp float64
	projection := make([]map[string]any, 0, len(events))
	for index, raw := range events {
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, fmt.Errorf("parse Gemini SSE event %d: %w", index, err)
		}
		name, _ := data["type"].(string)
		if name != expected[index] {
			return nil, fmt.Errorf("Gemini native SSE event %d is %q, want %q", index, name, expected[index])
		}
		if data["sequence_number"] != float64(index) {
			return nil, fmt.Errorf("Gemini native SSE sequence %d not sequential: %+v", index, data)
		}
		createdAt, valid := data["created_at"].(float64)
		if !valid || createdAt < 1 || (index != 0 && createdAt != timestamp) {
			return nil, fmt.Errorf("Gemini native SSE created_at inconsistent: %+v", data)
		}
		if index == 0 {
			timestamp = createdAt
		}
		response, ok := data["response"].(map[string]any)
		if !ok {
			return nil, errors.New("Gemini native SSE event missing response")
		}
		id, _ := response["id"].(string)
		if id == "" || (index != 0 && id != responseID) {
			return nil, fmt.Errorf("Gemini native SSE response ID changed: %+v", response)
		}
		if index == 0 {
			responseID = id
			if len(response) != 1 {
				return nil, fmt.Errorf("Gemini native SSE created response has extra fields: %+v", response)
			}
		} else {
			output, ok := response["output"].([]any)
			if !ok || len(output) != 0 || len(response) != 2 {
				return nil, fmt.Errorf("Gemini native chunks leaked output or completion metadata: %+v", response)
			}
		}
		// Validate SSE event labels independently from the JSON type field.
		label := "event: " + name
		if strings.Count(wire, label) != 1 {
			return nil, fmt.Errorf("Gemini SSE event label %q missing or duplicated", label)
		}
		projected := make(map[string]any, len(data))
		for key, value := range data {
			projected[key] = value
		}
		delete(projected, "created_at")
		projectedResponse := make(map[string]any, len(response))
		for key, value := range response {
			projectedResponse[key] = value
		}
		delete(projectedResponse, "id")
		projected["response"] = projectedResponse
		projection = append(projection, projected)
	}
	if strings.Contains(wire, "synthetic-") ||
		strings.Contains(wire, "event: response.output_text.delta") ||
		strings.Contains(wire, "event: response.failed") {
		return nil, errors.New("Gemini native chunks were incorrectly forwarded as Chat Completions output")
	}
	return projection, nil
}

func geminiNativeSSEEqual043711(a, b string) bool {
	left, leftErr := geminiNativeSSEProjection043711(a)
	right, rightErr := geminiNativeSSEProjection043711(b)
	return leftErr == nil && rightErr == nil && reflect.DeepEqual(left, right)
}
