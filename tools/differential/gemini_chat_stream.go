package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

var geminiChatSSEExpected043711 = []string{
	"response.created",
	"response.output_item.added",
	"response.output_text.delta",
	"response.output_text.delta",
	"response.output_item.done",
	"response.completed",
}

// The official Prodex 0.437.11 Gemini API-key streaming trace includes a
// created response, a created assistant output item, text deltas carrying
// response ID and timestamp, the completed item, and the terminal response
// with usage/finish metadata. Only random IDs and observed times are
// normalized, after validating their consistency across all six events.
func geminiChatSSEProjection043711(wire string) ([]map[string]any, error) {
	decoder := sse.NewDecoder(1 << 20)
	chunks := decoder.Feed([]byte(wire))
	chunks = append(chunks, decoder.Finish()...)
	if len(chunks) != len(geminiChatSSEExpected043711) {
		return nil, fmt.Errorf("Chat Gemini SSE emitted %d events, want six", len(chunks))
	}
	var responseID, itemID string
	var createdAt float64
	var rendered strings.Builder
	result := make([]map[string]any, 0, len(chunks))
	for i, chunk := range chunks {
		var event map[string]any
		if err := json.Unmarshal(chunk, &event); err != nil {
			return nil, fmt.Errorf("decode Chat Gemini SSE event %d: %w", i, err)
		}
		kind, _ := event["type"].(string)
		if kind != geminiChatSSEExpected043711[i] ||
			event["sequence_number"] != float64(i) {
			return nil, fmt.Errorf("Chat Gemini SSE event %d type or sequence differs: %+v", i, event)
		}
		if i == 0 {
			value, ok := event["created_at"].(float64)
			if !ok || value <= 0 {
				return nil, errors.New("Gemini Chat SSE initial timestamp missing")
			}
			createdAt = value
		}
		switch i {
		case 0:
			response, ok := event["response"].(map[string]any)
			if !ok {
				return nil, errors.New("Gemini Chat SSE missing created response")
			}
			responseID, _ = response["id"].(string)
			if !strings.HasPrefix(responseID, "resp_deepseek_") || len(response) != 1 {
				return nil, fmt.Errorf("Gemini Chat SSE created response ID/shape incorrect: %+v", response)
			}
		case 1:
			item, ok := event["item"].(map[string]any)
			if !ok {
				return nil, errors.New("Gemini Chat SSE missing created item")
			}
			itemID, _ = item["id"].(string)
			contents, ok := item["content"].([]any)
			if !strings.HasPrefix(itemID, "msg_gemini_") ||
				item["role"] != "assistant" || item["type"] != "message" ||
				!ok || len(contents) != 0 ||
				event["response_id"] != responseID {
				return nil, fmt.Errorf("Gemini Chat SSE output item identity invalid: %+v", event)
			}
		case 2, 3:
			value, _ := event["delta"].(string)
			expected := "synthetic-"
			if i == 3 {
				expected = "ok<>&/🌍\u2028\u2029"
			}
			if value != expected || event["created_at"] != createdAt ||
				event["response_id"] != responseID {
				return nil, fmt.Errorf("Gemini Chat SSE text delta/context invalid: %+v", event)
			}
			rendered.WriteString(value)
		case 4:
			item, ok := event["item"].(map[string]any)
			if !ok || item["id"] != itemID || item["role"] != "assistant" ||
				item["type"] != "message" || event["response_id"] != responseID ||
				!geminiChatTextItem043711(item["content"], rendered.String()) {
				return nil, fmt.Errorf("Gemini Chat SSE completed item invalid: %+v", event)
			}
		case 5:
			completed, ok := event["response"].(map[string]any)
			if !ok || completed["id"] != responseID ||
				event["created_at"] != createdAt ||
				completed["model"] != "gemini-3.1-pro-preview" {
				return nil, fmt.Errorf("Gemini Chat SSE completed response identity/model invalid: %+v", event)
			}
			output, _ := completed["output"].([]any)
			if len(output) != 1 {
				return nil, fmt.Errorf("Gemini Chat SSE completed response missing output: %+v", completed)
			}
			item, _ := output[0].(map[string]any)
			if item["role"] != "assistant" || item["type"] != "message" ||
				!geminiChatTextItem043711(item["content"], rendered.String()) {
				return nil, fmt.Errorf("Gemini Chat SSE response output changed: %+v", completed)
			}
			usage, _ := completed["usage"].(map[string]any)
			metadata, _ := completed["metadata"].(map[string]any)
			gemini, _ := metadata["gemini"].(map[string]any)
			if usage == nil || gemini == nil ||
				usage["input_tokens"] != float64(9007199254740993) ||
				usage["output_tokens"] != float64(1) ||
				usage["total_tokens"] != float64(9007199254740994) ||
				gemini["finish_reason"] != "stop" {
				return nil, fmt.Errorf("Gemini Chat SSE lost finish reason or token usage: %+v", completed)
			}
		}
		// Keep all material fields in the comparison; only the validated
		// generated IDs and clock timestamps are removed.
		normalized := make(map[string]any, len(event))
		for key, value := range event {
			normalized[key] = value
		}
		delete(normalized, "created_at")
		delete(normalized, "response_id")
		if response, ok := event["response"].(map[string]any); ok {
			copyValue := make(map[string]any, len(response))
			for key, value := range response {
				copyValue[key] = value
			}
			delete(copyValue, "id")
			normalized["response"] = copyValue
		}
		if item, ok := event["item"].(map[string]any); ok {
			copyValue := make(map[string]any, len(item))
			for key, value := range item {
				copyValue[key] = value
			}
			delete(copyValue, "id")
			normalized["item"] = copyValue
		}
		result = append(result, normalized)
	}
	for _, kind := range []string{
		"response.created", "response.output_item.added",
		"response.output_text.delta", "response.output_item.done", "response.completed",
	} {
		want := 1
		if kind == "response.output_text.delta" {
			want = 2
		}
		if strings.Count(wire, "event: "+kind) != want {
			return nil, fmt.Errorf("Gemini Chat SSE event framing for %s: want %d", kind, want)
		}
	}
	if wire != geminiChatSSEFixture043711(responseID, itemID, int64(createdAt)) {
		return nil, errors.New("Gemini Chat SSE bytes differ from the canonical released trace")
	}
	return result, nil
}

func geminiChatTextItem043711(input any, text string) bool {
	contents, ok := input.([]any)
	if !ok || len(contents) != 1 {
		return false
	}
	part, ok := contents[0].(map[string]any)
	return ok && part["type"] == "output_text" && part["text"] == text &&
		len(part) == 2
}

func geminiChatSSEEqual043711(first, second string) bool {
	a, errA := geminiChatSSEProjection043711(first)
	b, errB := geminiChatSSEProjection043711(second)
	return errA == nil && errB == nil && reflect.DeepEqual(a, b)
}
