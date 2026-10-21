package deepseek

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	redacthelper "github.com/christiandoxa/godex/internal/helper/redact"
	"github.com/christiandoxa/godex/internal/helper/sse"
)

type deepSeekChatStreamToolCall struct {
	id               string
	name             string
	arguments        strings.Builder
	thoughtSignature string
	finalArguments   string
	added            bool
	done             bool
}

type deepSeekChatStreamState struct {
	requestID            uint64
	sequenceNumber       uint64
	responseID           string
	model                string
	modelPresent         bool
	createdAt            uint64
	created              bool
	completed            bool
	json                 sse.JSONFrames
	outputText           strings.Builder
	reasoning            strings.Builder
	refusal              strings.Builder
	finishReason         string
	systemFingerprint    string
	usage                any
	usagePresent         bool
	logprobs             any
	annotations          []any
	outputTextItemAdded  bool
	outputTextItemDone   bool
	reasoningPartAdded   bool
	toolCalls            map[uint64]*deepSeekChatStreamToolCall
	conversationMessages []any
	conversations        deepSeekConversationStore
	responseMetadata     map[string]any
}

type deepSeekCompletedStreamTool struct {
	index uint64
	item  map[string]any
}

func newDeepSeekChatStreamState(requestID uint64, messages []any, metadata map[string]any, conversations deepSeekConversationStore) *deepSeekChatStreamState {
	return &deepSeekChatStreamState{
		requestID: requestID, responseID: deepSeekResponseFallbackID(), createdAt: uint64(time.Now().Unix()),
		toolCalls: make(map[uint64]*deepSeekChatStreamToolCall), conversationMessages: cloneDeepSeekMessages(messages),
		conversations: conversations, responseMetadata: metadata,
	}
}

func (state *deepSeekChatStreamState) observeValue(data []byte) ([]byte, bool, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, false, fmt.Errorf("failed to parse DeepSeek SSE JSON")
	}
	root, _ := value.(map[string]any)
	if value, present := root["error"]; present {
		code, message := chatcompat.ChatStreamError(value)
		return state.failed(code, message)
	}
	state.applyChunkMetadata(root)

	var events [][]byte
	if !state.created {
		state.created = true
		events = append(events, state.createdEvent())
	}
	choice := firstDeepSeekStreamChoice(root)
	if choice == nil {
		return bytes.Join(events, nil), len(events) > 0, nil
	}
	if logprobs, ok := choice["logprobs"]; ok && logprobs != nil {
		state.logprobs = cloneMetadataValue(logprobs)
	}
	if finish, _ := choice["finish_reason"].(string); strings.TrimSpace(finish) != "" {
		state.finishReason = finish
	}

	delta, _ := choice["delta"].(map[string]any)
	if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
		state.reasoning.WriteString(reasoning)
		if !state.reasoningPartAdded {
			state.reasoningPartAdded = true
			events = append(events, anthropicStreamEvent("response.reasoning_summary_part.added", map[string]any{
				"type": "response.reasoning_summary_part.added", "sequence_number": state.nextSequenceNumber(),
				"response_id": state.responseID, "output_index": 0, "summary_index": 0,
				"part": map[string]any{"type": "summary_text", "text": ""},
			}))
		}
		events = append(events, anthropicStreamEvent("response.reasoning_summary_text.delta", map[string]any{
			"type": "response.reasoning_summary_text.delta", "sequence_number": state.nextSequenceNumber(),
			"response_id": state.responseID, "output_index": 0, "summary_index": 0, "delta": reasoning,
		}))
	}
	if refusal, ok := delta["refusal"].(string); ok && refusal != "" {
		state.refusal.WriteString(refusal)
	}
	if annotations, ok := delta["annotations"].([]any); ok && len(annotations) > 0 {
		state.annotations = append(state.annotations, cloneDeepSeekMessages(annotations)...)
	}
	if text, ok := delta["content"].(string); ok && text != "" {
		if !state.outputTextItemAdded {
			state.outputTextItemAdded = true
			events = append(events, state.outputTextItemAddedEvent())
		}
		state.outputText.WriteString(text)
		events = append(events, anthropicStreamEvent("response.output_text.delta", map[string]any{
			"type": "response.output_text.delta", "sequence_number": state.nextSequenceNumber(),
			"created_at": state.createdAt, "response_id": state.responseID, "delta": text,
		}))
	}
	if calls, ok := delta["tool_calls"].([]any); ok {
		for _, raw := range calls {
			call, _ := raw.(map[string]any)
			if call == nil {
				continue
			}
			toolEvents, failed := state.observeToolCallDelta(call)
			events = append(events, toolEvents...)
			if failed {
				return bytes.Join(events, nil), true, nil
			}
		}
	}
	if state.finishReason != "" && len(state.toolCalls) > 0 && !state.completed {
		state.storeConversationSnapshot()
	}
	return bytes.Join(events, nil), len(events) > 0, nil
}

func (state *deepSeekChatStreamState) complete() ([]byte, bool, error) {
	if state.completed {
		return nil, false, nil
	}
	for index, call := range state.toolCalls {
		if err := deepSeekValidateStreamToolCall(index, call); err != nil {
			return state.failed("invalid_tool_call_arguments", err.Error())
		}
	}

	completedTools := state.completedToolItems()
	baseAssistant := map[string]any{"role": "assistant"}
	if state.outputText.Len() > 0 {
		baseAssistant["content"] = state.outputText.String()
	}
	if state.reasoning.Len() > 0 {
		baseAssistant["reasoning_content"] = state.reasoning.String()
	}
	if state.refusal.Len() > 0 {
		baseAssistant["refusal"] = state.refusal.String()
	}
	if len(state.annotations) > 0 {
		baseAssistant["annotations"] = cloneDeepSeekMessages(state.annotations)
	}
	choice := map[string]any{"message": baseAssistant}
	if state.finishReason != "" {
		choice["finish_reason"] = state.finishReason
	}
	if state.logprobs != nil {
		choice["logprobs"] = cloneMetadataValue(state.logprobs)
	}
	source := map[string]any{
		"id": state.responseID, "model": state.model, "created": state.createdAt,
		"choices": []any{choice},
	}
	if state.systemFingerprint != "" {
		source["system_fingerprint"] = state.systemFingerprint
	}
	if state.usagePresent {
		source["usage"] = state.usage
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return nil, false, err
	}
	// The source carries its unsigned epoch; the fallback clock is never used.
	translated, err := deepSeekChatResponse(encoded, time.Unix(0, 0))
	if err != nil {
		return nil, false, err
	}
	var response map[string]any
	decoder := json.NewDecoder(bytes.NewReader(translated))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, false, err
	}
	output := make([]any, 0, len(completedTools)+1)
	if state.outputText.Len() > 0 {
		output = append(output, map[string]any{
			"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": state.outputText.String()}},
		})
	}
	for _, tool := range completedTools {
		output = append(output, tool.item)
	}
	response["output"] = output
	mergeResponseMetadata(response, state.responseMetadata)
	// The tagged Prodex DeepSeek SSE completion value is a sparse stream
	// projection, not the buffered Responses object. created_at belongs to
	// the event envelope; object and created_at are absent from response.
	delete(response, "object")
	delete(response, "created_at")
	if !state.modelPresent {
		delete(response, "model")
	}

	var events [][]byte
	for _, tool := range completedTools {
		events = append(events, anthropicStreamEvent("response.output_item.done", map[string]any{
			"type": "response.output_item.done", "sequence_number": state.nextSequenceNumber(),
			"output_index": tool.index, "item": tool.item,
		}))
	}
	if state.reasoning.Len() > 0 {
		events = append(events, anthropicStreamEvent("response.reasoning_summary_text.done", map[string]any{
			"type": "response.reasoning_summary_text.done", "sequence_number": state.nextSequenceNumber(),
			"response_id": state.responseID, "output_index": 0, "summary_index": 0, "text": state.reasoning.String(),
		}))
	}
	if state.outputText.Len() > 0 && !state.outputTextItemDone {
		// Prodex 0.436.1 completes the message item directly. Emitting
		// response.output_text.done here creates an extra client-visible
		// event and shifts all subsequent sequence numbers.
		state.outputTextItemDone = true
		events = append(events, state.outputTextItemDoneEvent())
	}
	events = append(events, anthropicStreamEvent("response.completed", map[string]any{
		"type": "response.completed", "sequence_number": state.nextSequenceNumber(),
		"created_at": state.createdAt, "response": response,
	}))
	state.storeConversationSnapshot()
	state.completed = true
	return bytes.Join(events, nil), true, nil
}

func (state *deepSeekChatStreamState) completedToolItems() []deepSeekCompletedStreamTool {
	indices := make([]uint64, 0, len(state.toolCalls))
	for index := range state.toolCalls {
		indices = append(indices, index)
	}
	sort.Slice(indices, func(left, right int) bool { return indices[left] < indices[right] })
	items := make([]deepSeekCompletedStreamTool, 0, len(indices))
	for _, index := range indices {
		call := state.toolCalls[index]
		if call == nil || call.done {
			continue
		}
		call.done = true
		item := deepSeekStreamCompletedToolItem(call)
		if item != nil {
			items = append(items, deepSeekCompletedStreamTool{index: index, item: item})
		}
	}
	return items
}

func (state *deepSeekChatStreamState) failed(code, message string) ([]byte, bool, error) {
	if state.completed {
		return nil, false, nil
	}
	state.completed = true
	return anthropicStreamEvent("response.failed", map[string]any{
		"type": "response.failed", "sequence_number": state.nextSequenceNumber(),
		"created_at": state.createdAt,
		"response": map[string]any{
			"id":    state.responseID,
			"error": map[string]any{"code": code, "message": redacthelper.Secrets(message)},
		},
	}), true, nil
}

func (state *deepSeekChatStreamState) createdEvent() []byte {
	return anthropicStreamEvent("response.created", map[string]any{
		"type": "response.created", "sequence_number": state.nextSequenceNumber(),
		"created_at": state.createdAt, "response": map[string]any{"id": state.responseID},
	})
}

func (state *deepSeekChatStreamState) assistantMessage() map[string]any {
	assistant := map[string]any{"role": "assistant"}
	if state.outputText.Len() > 0 {
		assistant["content"] = state.outputText.String()
	} else if len(state.toolCalls) > 0 {
		assistant["content"] = ""
	} else {
		assistant["content"] = nil
	}
	if state.reasoning.Len() > 0 {
		assistant["reasoning_content"] = state.reasoning.String()
	}
	if len(state.toolCalls) > 0 {
		indices := make([]uint64, 0, len(state.toolCalls))
		for index := range state.toolCalls {
			indices = append(indices, index)
		}
		sort.Slice(indices, func(left, right int) bool { return indices[left] < indices[right] })
		calls := make([]any, 0, len(indices))
		for _, index := range indices {
			call := state.toolCalls[index]
			if call == nil {
				continue
			}
			arguments := call.finalArguments
			if arguments == "" {
				arguments = call.arguments.String()
			}
			value := map[string]any{
				"id": call.id, "type": "function",
				"function": map[string]any{"name": call.name, "arguments": arguments},
			}
			if call.thoughtSignature != "" {
				value["extra_content"] = map[string]any{"google": map[string]any{"thought_signature": call.thoughtSignature}}
			}
			calls = append(calls, value)
		}
		assistant["tool_calls"] = calls
	}
	return assistant
}

func (state *deepSeekChatStreamState) storeConversationSnapshot() {
	if strings.TrimSpace(state.responseID) == "" {
		return
	}
	assistant := state.assistantMessage()
	if assistant["content"] == nil && len(state.toolCalls) == 0 && state.reasoning.Len() == 0 {
		return
	}
	messages := cloneDeepSeekMessages(state.conversationMessages)
	messages = append(messages, assistant)
	state.conversations.insert(state.responseID, messages)
}

func (state *deepSeekChatStreamState) nextSequenceNumber() uint64 {
	current := state.sequenceNumber
	state.sequenceNumber++
	return current
}
