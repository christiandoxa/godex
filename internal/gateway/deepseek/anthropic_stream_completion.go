package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	redacthelper "github.com/christiandoxa/godex/internal/helper/redact"
)

func (state *anthropicStreamState) outputDoneEvents() ([][]byte, error) {
	var searches, tools, textItems [][]byte
	for _, block := range state.blockOrder {
		if block.done {
			continue
		}
		switch block.kind {
		case "server_tool_use":
			item, err := anthropicResponseWebSearch(block.value)
			if err != nil {
				return nil, err
			}
			item["action"].(map[string]any)["sources"] = block.sources
			searches = append(searches, state.streamItemDone(block.index, item))
			block.done = true
		case "tool_use":
			item, err := anthropicResponseToolUse(block.value)
			if err != nil {
				return nil, err
			}
			tools = append(tools, state.streamItemDone(block.index, item))
			block.done = true
		case "text":
			text := stringOr(block.value["text"], "")
			if block.added && text != "" {
				item := map[string]any{
					"type": "message", "role": "assistant",
					"content": []any{map[string]any{"type": "output_text", "text": text}},
				}
				item["id"] = state.outputTextItemID()
				textItems = append(textItems, anthropicStreamEvent("response.output_item.done", map[string]any{
					"type": "response.output_item.done", "sequence_number": state.nextSequenceNumber(),
					"response_id": state.id, "item": item,
				}))
			}
			block.done = true
		}
	}
	events := append(searches, tools...)
	if text := state.reasoningText.String(); text != "" {
		events = append(events, anthropicStreamEvent("response.reasoning_summary_text.done", map[string]any{
			"type": "response.reasoning_summary_text.done", "sequence_number": state.nextSequenceNumber(),
			"response_id": state.id, "output_index": 0, "summary_index": 0, "text": text,
		}))
	}
	return append(events, textItems...), nil
}

func (state *anthropicStreamState) finishWebSearch(result map[string]any) ([]byte, bool, error) {
	toolUseID, ok := result["tool_use_id"].(string)
	if !ok {
		return nil, false, nil
	}
	for _, block := range state.blockOrder {
		if block.kind != "server_tool_use" || block.value["id"] != toolUseID || block.done {
			continue
		}
		if block.partialJSON.Len() > 0 {
			var input any
			if err := json.Unmarshal([]byte(block.partialJSON.String()), &input); err != nil {
				input = map[string]any{}
			}
			block.value["input"] = input
		}
		block.sources = anthropicWebSearchSources(result)
		item, err := anthropicResponseWebSearch(block.value)
		if err != nil {
			return nil, false, err
		}
		item["action"].(map[string]any)["sources"] = block.sources
		block.done = true
		return state.streamItemDone(block.index, item), true, nil
	}
	return nil, false, nil
}

func (state *anthropicStreamState) streamItemDone(index uint64, item map[string]any) []byte {
	return anthropicStreamEvent("response.output_item.done", map[string]any{
		"type": "response.output_item.done", "sequence_number": state.nextSequenceNumber(),
		"output_index": index, "item": item,
	})
}

func (state *anthropicStreamState) failed(code, message string, now time.Time) ([]byte, bool, error) {
	if state.completed {
		return nil, false, nil
	}
	if state.id == "" {
		state.id = anthropicStreamFallbackID(now)
	}
	if state.createdAt.IsZero() {
		state.createdAt = now
	}
	state.completed = true
	return anthropicStreamEvent("response.failed", map[string]any{
		"type": "response.failed", "sequence_number": state.nextSequenceNumber(),
		"created_at": state.createdAt.Unix(),
		"response": map[string]any{
			"id":    state.id,
			"error": map[string]any{"code": code, "message": redacthelper.Secrets(message)},
		},
	}), true, nil
}

func anthropicStreamFallbackID(time.Time) string {
	return deepSeekResponseFallbackID()
}

func (state *anthropicStreamState) messageStopped(now time.Time) ([]byte, bool, error) {
	if state.completed {
		return nil, false, nil
	}
	content := make([]any, 0, len(state.blockOrder))
	for _, block := range state.blockOrder {
		if block.kind == "text" && stringOr(block.value["text"], "") == "" {
			continue
		}
		if block.partialJSON.Len() > 0 {
			var input any
			if err := json.Unmarshal([]byte(block.partialJSON.String()), &input); err != nil {
				if block.kind != "server_tool_use" {
					return nil, false, fmt.Errorf("failed to parse Anthropic tool input JSON: %w", err)
				}
				input = map[string]any{}
			}
			block.value["input"] = input
		}
		content = append(content, block.value)
	}
	createdAt := state.createdAt
	if createdAt.IsZero() {
		createdAt = now
	}
	if state.id == "" {
		state.id = anthropicStreamFallbackID(now)
	}
	usage := map[string]any{"input_tokens": state.inputTokens, "output_tokens": state.outputTokens}
	if state.serverToolUse != nil {
		usage["server_tool_use"] = state.serverToolUse
	}
	responseSource := map[string]any{
		"id": state.id, "model": state.model, "content": content, "usage": usage,
	}
	if state.hasStopReason {
		responseSource["stop_reason"] = state.stopReason
	}
	encoded, err := json.Marshal(responseSource)
	if err != nil {
		return nil, false, errors.New("failed to serialize native DeepSeek Messages stream response")
	}
	completed, err := deepSeekAnthropicResponse(encoded, createdAt)
	if err != nil {
		return nil, false, err
	}
	completed, err = mergeAnthropicResponseMetadata(completed, state.requestMetadata)
	if err != nil {
		return nil, false, err
	}
	var response map[string]any
	if err := json.Unmarshal(completed, &response); err != nil {
		return nil, false, errors.New("failed to parse completed native DeepSeek Messages response")
	}
	events, err := state.outputDoneEvents()
	if err != nil {
		return nil, false, err
	}
	events = append(events, anthropicStreamEvent("response.completed", map[string]any{
		"type": "response.completed", "sequence_number": state.nextSequenceNumber(),
		"created_at": createdAt.Unix(), "response": response,
	}))
	if encodedSource, encodeErr := json.Marshal(responseSource); encodeErr == nil {
		messages := chatcompat.CloneMessages(state.conversationMessages)
		messages = append(messages, deepSeekAnthropicAssistantMessages(encodedSource)...)
		state.conversations.Insert(state.id, messages)
	}
	state.completed = true
	return bytes.Join(events, nil), true, nil
}
