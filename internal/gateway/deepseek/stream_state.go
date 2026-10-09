package deepseek

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	redacthelper "github.com/christiandoxa/godex/internal/helper/redact"
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
	createdAt            time.Time
	created              bool
	completed            bool
	outputText           strings.Builder
	reasoning            strings.Builder
	refusal              strings.Builder
	finishReason         string
	systemFingerprint    string
	usage                any
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
		requestID: requestID, responseID: deepSeekResponseFallbackID(), createdAt: time.Now(),
		toolCalls: make(map[uint64]*deepSeekChatStreamToolCall), conversationMessages: cloneDeepSeekMessages(messages),
		conversations: conversations, responseMetadata: metadata,
	}
}

func (state *deepSeekChatStreamState) observe(data []byte) ([]byte, bool, error) {
	if state.completed {
		return nil, false, nil
	}
	if string(data) == "[DONE]" {
		return state.complete()
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil, false, fmt.Errorf("failed to parse DeepSeek SSE JSON")
	}
	if code, message, ok := deepSeekEmbeddedStreamError(root); ok {
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
			"created_at": state.createdAt.Unix(), "response_id": state.responseID, "delta": text,
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

func (state *deepSeekChatStreamState) applyChunkMetadata(root map[string]any) {
	if id, _ := root["id"].(string); strings.TrimSpace(id) != "" && strings.HasPrefix(state.responseID, "resp_deepseek_") {
		state.responseID = strings.TrimSpace(id)
	}
	if model, _ := root["model"].(string); strings.TrimSpace(model) != "" {
		state.model = strings.TrimSpace(model)
	}
	if created, ok := deepSeekStreamUint(root["created"]); ok {
		state.createdAt = time.Unix(int64(created), 0)
	}
	if fingerprint, _ := root["system_fingerprint"].(string); fingerprint != "" {
		state.systemFingerprint = fingerprint
	}
	if usage, ok := root["usage"].(map[string]any); ok {
		state.usage = cloneDeepSeekMap(usage)
	}
}

func deepSeekStreamUint(value any) (uint64, bool) {
	switch current := value.(type) {
	case float64:
		if current >= 0 && current == float64(uint64(current)) {
			return uint64(current), true
		}
	case json.Number:
		parsed, err := strconv.ParseUint(current.String(), 10, 64)
		return parsed, err == nil
	case uint64:
		return current, true
	case int:
		if current >= 0 {
			return uint64(current), true
		}
	}
	return 0, false
}

func deepSeekEmbeddedStreamError(root map[string]any) (string, string, bool) {
	raw, ok := root["error"]
	if !ok {
		return "", "", false
	}
	if message, ok := raw.(string); ok {
		return "provider_stream_error", message, true
	}
	errorObject, _ := raw.(map[string]any)
	code := "provider_stream_error"
	for _, key := range []string{"type", "code"} {
		switch value := errorObject[key].(type) {
		case string:
			if value != "" {
				code = value
				goto codeDone
			}
		case float64:
			if value == float64(int64(value)) {
				code = strconv.FormatInt(int64(value), 10)
				goto codeDone
			}
		case json.Number:
			code = value.String()
			goto codeDone
		}
	}
codeDone:
	message := "Provider stream returned an embedded error"
	for _, key := range []string{"message", "detail"} {
		if value, _ := errorObject[key].(string); value != "" {
			message = value
			break
		}
	}
	return code, message, true
}

func (state *deepSeekChatStreamState) observeToolCallDelta(call map[string]any) ([][]byte, bool) {
	index := integerValue(call["index"])
	current := state.toolCalls[index]
	function, hasFunction := call["function"].(map[string]any)
	if current == nil && !hasFunction {
		failed, _, _ := state.failed("invalid_tool_call_arguments", "DeepSeek streamed a tool call without a function object")
		return [][]byte{failed}, true
	}
	if current == nil {
		current = &deepSeekChatStreamToolCall{id: deepSeekCallFallbackID()}
		state.toolCalls[index] = current
	}
	if id, _ := call["id"].(string); strings.TrimSpace(id) != "" {
		current.id = strings.TrimSpace(id)
	}
	if hasFunction {
		if name, _ := function["name"].(string); strings.TrimSpace(name) != "" {
			current.name = strings.TrimSpace(name)
		}
	}
	if signature := deepSeekStreamThoughtSignature(call); signature != "" {
		current.thoughtSignature = signature
	}

	name := current.name
	if name == "" {
		name = "tool_call"
	}
	var events [][]byte
	if !current.added {
		current.added = true
		if item := deepSeekStreamAddedToolItem(current.id, name); item != nil {
			events = append(events, anthropicStreamEvent("response.output_item.added", map[string]any{
				"type": "response.output_item.added", "sequence_number": state.nextSequenceNumber(),
				"output_index": index, "item": item,
			}))
		}
	}
	if hasFunction {
		if arguments, ok := function["arguments"].(string); ok && arguments != "" {
			current.arguments.WriteString(arguments)
			if name != "tool_search" && name != "apply_patch" {
				events = append(events, anthropicStreamEvent("response.function_call_arguments.delta", map[string]any{
					"type": "response.function_call_arguments.delta", "sequence_number": state.nextSequenceNumber(),
					"output_index": index, "call_id": current.id, "delta": arguments,
				}))
			}
		}
	}
	return events, false
}

func deepSeekStreamThoughtSignature(call map[string]any) string {
	extra, _ := call["extra_content"].(map[string]any)
	google, _ := extra["google"].(map[string]any)
	signature, _ := google["thought_signature"].(string)
	return strings.TrimSpace(signature)
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
		"id": state.responseID, "model": state.model, "created": state.createdAt.Unix(),
		"choices": []any{choice},
	}
	if state.systemFingerprint != "" {
		source["system_fingerprint"] = state.systemFingerprint
	}
	if state.usage != nil {
		source["usage"] = cloneMetadataValue(state.usage)
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return nil, false, err
	}
	translated, err := deepSeekChatResponse(encoded, state.createdAt)
	if err != nil {
		return nil, false, err
	}
	var response map[string]any
	if err := json.Unmarshal(translated, &response); err != nil {
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
		"created_at": state.createdAt.Unix(), "response": response,
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
		"created_at": state.createdAt.Unix(),
		"response": map[string]any{
			"id":    state.responseID,
			"error": map[string]any{"code": code, "message": redacthelper.Secrets(message)},
		},
	}), true, nil
}

func (state *deepSeekChatStreamState) createdEvent() []byte {
	return anthropicStreamEvent("response.created", map[string]any{
		"type": "response.created", "sequence_number": state.nextSequenceNumber(),
		"created_at": state.createdAt.Unix(), "response": map[string]any{"id": state.responseID},
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

func firstDeepSeekStreamChoice(root map[string]any) map[string]any {
	choices, _ := root["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	return choice
}
