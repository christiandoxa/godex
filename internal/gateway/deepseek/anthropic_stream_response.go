package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type anthropicStreamState struct {
	requestID            uint64
	sequenceNumber       uint64
	id                   string
	model                string
	createdAt            time.Time
	blocks               map[uint64]*anthropicStreamBlock
	blockOrder           []*anthropicStreamBlock
	inputTokens          uint64
	outputTokens         uint64
	serverToolUse        map[string]any
	reasoningText        strings.Builder
	reasoningAdded       bool
	stopReason           any
	hasStopReason        bool
	requestMetadata      map[string]any
	conversationMessages []any
	conversations        deepSeekConversationStore
	completed            bool
}

type anthropicStreamBlock struct {
	kind        string
	index       uint64
	value       map[string]any
	partialJSON strings.Builder
	sources     []any
	added       bool
	done        bool
}

func (state *anthropicStreamState) translate(data []byte, now time.Time) ([]byte, bool, error) {
	if state.createdAt.IsZero() {
		state.createdAt = now
	}
	if state.id == "" {
		state.id = anthropicStreamFallbackID(now)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil, false, errors.New("failed to parse Anthropic SSE JSON")
	}
	switch root["type"] {
	case "message_start":
		return state.messageStarted(root, now)
	case "content_block_start":
		return state.blockStarted(root)
	case "content_block_delta":
		return state.blockDelta(root)
	case "message_delta":
		state.observeUsage(root["usage"])
		delta, _ := root["delta"].(map[string]any)
		if reason, found := delta["stop_reason"]; found {
			state.stopReason, state.hasStopReason = reason, true
		}
		return nil, false, nil
	case "message_stop":
		return state.messageStopped(now)
	case "error":
		providerError, _ := root["error"].(map[string]any)
		return state.failed(
			stringOr(providerError["type"], "anthropic_stream_error"),
			stringOr(providerError["message"], "Anthropic stream failed"),
			now,
		)
	default:
		return nil, false, nil
	}
}

func (state *anthropicStreamState) messageStarted(root map[string]any, now time.Time) ([]byte, bool, error) {
	message, _ := root["message"].(map[string]any)
	if id, ok := message["id"].(string); ok {
		state.id = id
	}
	state.model = stringOr(message["model"], "unknown")
	state.observeUsage(message["usage"])
	return anthropicStreamEvent("response.created", map[string]any{
		"type": "response.created", "sequence_number": state.nextSequenceNumber(),
		"created_at": state.createdAt.Unix(), "response": map[string]any{"id": state.id},
	}), true, nil
}

func (state *anthropicStreamState) blockStarted(root map[string]any) ([]byte, bool, error) {
	block, ok := root["content_block"].(map[string]any)
	if !ok {
		return nil, false, errors.New("Anthropic content_block_start requires content_block")
	}
	kind, ok := block["type"].(string)
	if !ok {
		return nil, false, errors.New("Anthropic content block requires type")
	}
	if !supportedAnthropicStreamBlock(kind, block) {
		return nil, false, nil
	}
	index := integerValue(root["index"])
	streamBlock := state.rememberStreamBlock(index, kind, block)
	if kind == "text" {
		streamBlock.value["text"] = ""
	}
	if kind == "web_search_tool_result" {
		return state.finishWebSearch(block)
	}
	item := state.streamBlockStartItem(streamBlock)
	if item == nil {
		return nil, false, nil
	}
	streamBlock.added = true
	return anthropicStreamEvent("response.output_item.added", map[string]any{
		"type": "response.output_item.added", "sequence_number": state.nextSequenceNumber(),
		"output_index": index, "item": item,
	}), true, nil
}

func supportedAnthropicStreamBlock(kind string, block map[string]any) bool {
	switch kind {
	case "text", "thinking", "tool_use", "web_search_tool_result":
		return true
	case "server_tool_use":
		return block["name"] == "web_search"
	default:
		return false
	}
}

func (state *anthropicStreamState) rememberStreamBlock(index uint64, kind string, block map[string]any) *anthropicStreamBlock {
	if state.blocks == nil {
		state.blocks = make(map[uint64]*anthropicStreamBlock)
	}
	value := make(map[string]any, len(block))
	for key, item := range block {
		value[key] = item
	}
	streamBlock := &anthropicStreamBlock{kind: kind, index: index, value: value}
	state.blocks[index] = streamBlock
	state.blockOrder = append(state.blockOrder, streamBlock)
	return streamBlock
}

func (state *anthropicStreamState) streamBlockStartItem(block *anthropicStreamBlock) map[string]any {
	switch block.kind {
	case "tool_use":
		return map[string]any{
			"type": "function_call", "call_id": block.value["id"],
			"name": block.value["name"], "arguments": "",
		}
	case "server_tool_use":
		id, ok := block.value["id"].(string)
		if !ok {
			id = fmt.Sprintf("web_search_%d", block.index)
			block.value["id"] = id
		}
		return map[string]any{
			"type": "web_search_call", "id": id, "status": "in_progress",
			"action": map[string]any{
				"type": "search", "queries": anthropicStreamQueries(block.value["input"]), "sources": []any{},
			},
		}
	default:
		return nil
	}
}

func (state *anthropicStreamState) blockDelta(root map[string]any) ([]byte, bool, error) {
	delta, _ := root["delta"].(map[string]any)
	kind, ok := delta["type"].(string)
	if !ok {
		return nil, false, errors.New("Anthropic content_block_delta requires delta.type")
	}
	index := integerValue(root["index"])
	block := state.blocks[index]
	if block == nil && (kind == "text_delta" || kind == "thinking_delta") {
		blockKind := "text"
		if kind == "thinking_delta" {
			blockKind = "thinking"
		}
		block = state.ensureSyntheticBlock(index, blockKind)
	}
	switch kind {
	case "text_delta":
		return state.textBlockDelta(block, delta)
	case "input_json_delta":
		return state.inputJSONBlockDelta(index, block, delta)
	case "thinking_delta":
		return state.thinkingBlockDelta(block, delta)
	default:
		return nil, false, nil
	}
}

func (state *anthropicStreamState) textBlockDelta(block *anthropicStreamBlock, delta map[string]any) ([]byte, bool, error) {
	if block == nil || block.kind != "text" {
		return nil, false, nil
	}
	text := stringOr(delta["text"], "")
	if text == "" {
		return nil, false, nil
	}
	block.value["text"] = stringOr(block.value["text"], "") + text
	var events [][]byte
	if !block.added {
		block.added = true
		events = append(events, anthropicStreamEvent("response.output_item.added", map[string]any{
			"type": "response.output_item.added", "sequence_number": state.nextSequenceNumber(),
			"response_id": state.id,
			"item":        map[string]any{"id": state.outputTextItemID(), "type": "message", "role": "assistant", "content": []any{}},
		}))
	}
	events = append(events, anthropicStreamEvent("response.output_text.delta", map[string]any{
		"type": "response.output_text.delta", "sequence_number": state.nextSequenceNumber(),
		"created_at": state.createdAt.Unix(), "response_id": state.id, "delta": text,
	}))
	return bytes.Join(events, nil), true, nil
}

func (state *anthropicStreamState) inputJSONBlockDelta(index uint64, block *anthropicStreamBlock, delta map[string]any) ([]byte, bool, error) {
	text := stringOr(delta["partial_json"], "")
	if block != nil && (block.kind == "tool_use" || block.kind == "server_tool_use") {
		block.partialJSON.WriteString(text)
	}
	if block != nil && block.kind == "server_tool_use" {
		return nil, false, nil
	}
	payload := map[string]any{
		"type": "response.function_call_arguments.delta", "sequence_number": state.nextSequenceNumber(),
		"output_index": index, "delta": text,
	}
	if block != nil && block.kind == "tool_use" {
		if callID, ok := block.value["id"].(string); ok {
			payload["call_id"] = callID
		}
	}
	return anthropicStreamEvent("response.function_call_arguments.delta", payload), true, nil
}

func (state *anthropicStreamState) thinkingBlockDelta(block *anthropicStreamBlock, delta map[string]any) ([]byte, bool, error) {
	if block == nil || block.kind != "thinking" {
		return nil, false, nil
	}
	text := stringOr(delta["thinking"], "")
	if text == "" {
		return nil, false, nil
	}
	block.value["thinking"] = stringOr(block.value["thinking"], "") + text
	state.reasoningText.WriteString(text)
	var events [][]byte
	if !state.reasoningAdded {
		state.reasoningAdded = true
		events = append(events, anthropicStreamEvent("response.reasoning_summary_part.added", map[string]any{
			"type": "response.reasoning_summary_part.added", "sequence_number": state.nextSequenceNumber(),
			"response_id": state.id, "output_index": 0, "summary_index": 0,
			"part": map[string]any{"type": "summary_text", "text": ""},
		}))
	}
	events = append(events, anthropicStreamEvent("response.reasoning_summary_text.delta", map[string]any{
		"type": "response.reasoning_summary_text.delta", "sequence_number": state.nextSequenceNumber(),
		"response_id": state.id, "output_index": 0, "summary_index": 0, "delta": text,
	}))
	return bytes.Join(events, nil), true, nil
}

func (state *anthropicStreamState) ensureSyntheticBlock(index uint64, kind string) *anthropicStreamBlock {
	if state.blocks == nil {
		state.blocks = make(map[uint64]*anthropicStreamBlock)
	}
	if block := state.blocks[index]; block != nil {
		return block
	}
	value := map[string]any{"type": kind}
	if kind == "text" {
		value["text"] = ""
	} else if kind == "thinking" {
		value["thinking"] = ""
	}
	block := &anthropicStreamBlock{kind: kind, index: index, value: value}
	state.blocks[index] = block
	state.blockOrder = append(state.blockOrder, block)
	return block
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
		messages := cloneDeepSeekMessages(state.conversationMessages)
		messages = append(messages, deepSeekAnthropicAssistantMessages(encodedSource)...)
		state.conversations.insert(state.id, messages)
	}
	state.completed = true
	return bytes.Join(events, nil), true, nil
}

func (state *anthropicStreamState) observeUsage(value any) {
	usage, _ := value.(map[string]any)
	if input, found := usage["input_tokens"]; found {
		state.inputTokens = integerValue(input)
	}
	if output, found := usage["output_tokens"]; found {
		state.outputTokens = integerValue(output)
	}
	if serverToolUse, ok := usage["server_tool_use"].(map[string]any); ok {
		state.serverToolUse = serverToolUse
	}
}

func anthropicStreamQueries(value any) []any {
	input, _ := value.(map[string]any)
	if query, ok := input["query"].(string); ok {
		return []any{query}
	}
	queries, ok := input["queries"].([]any)
	if !ok {
		return []any{}
	}
	result := make([]any, 0, len(queries))
	for _, raw := range queries {
		if query, ok := raw.(string); ok {
			result = append(result, query)
		}
	}
	return result
}

func (state *anthropicStreamState) nextSequenceNumber() uint64 {
	value := state.sequenceNumber
	state.sequenceNumber++
	return value
}

func (state *anthropicStreamState) outputTextItemID() string {
	return fmt.Sprintf("msg_deepseek_%d", state.requestID)
}

func anthropicStreamEvent(name string, payload map[string]any) []byte {
	content, _ := json.Marshal(payload)
	return []byte(fmt.Sprintf("event: %s\r\ndata: %s\r\n\r\n", name, content))
}
