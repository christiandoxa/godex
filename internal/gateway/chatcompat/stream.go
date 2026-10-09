package chatcompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

const streamEventMaxBytes = 1 << 20
const streamMetadataMaxBytes = 1 << 20

const streamReasoningBuilderKey = "\x00reasoning_content_builder"
const streamCompletedKey = "\x00completed"

func ChatSSE(body io.ReadCloser) io.ReadCloser {
	return ChatSSEWithMetadata(body, "", nil)
}

func ChatSSEWithMetadata(body io.ReadCloser, providerKey string, responseMetadata map[string]any) io.ReadCloser {
	reader, writer := io.Pipe()
	go pumpChatSSE(body, writer, providerKey, responseMetadata)
	return reader
}

func pumpChatSSE(body io.ReadCloser, writer *io.PipeWriter, providerKey string, responseMetadata map[string]any) {
	defer body.Close()
	decoder := sse.NewDecoder(streamEventMaxBytes)
	buffer := make([]byte, 32<<10)
	metadata := make(map[string]any)
	for {
		read, err := body.Read(buffer)
		if read > 0 {
			if writeErr := writeTranslatedEvents(writer, decoder.Feed(buffer[:read]), providerKey, responseMetadata, metadata); writeErr != nil {
				closeChatSSEFailure(writer, metadata)
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if writeErr := writeTranslatedEvents(writer, decoder.Finish(), providerKey, responseMetadata, metadata); writeErr != nil {
					closeChatSSEFailure(writer, metadata)
					return
				}
				if completed, _ := metadata[streamCompletedKey].(bool); !completed {
					if _, writeErr := writer.Write(streamFailureEvent("unexpected end of stream")); writeErr != nil {
						_ = writer.CloseWithError(writeErr)
						return
					}
				}
				_ = writer.Close()
				return
			}
			if _, writeErr := writer.Write(streamFailureEvent("provider stream failed")); writeErr != nil {
				_ = writer.CloseWithError(writeErr)
				return
			}
			_ = writer.Close()
			return
		}
	}
}

func closeChatSSEFailure(writer *io.PipeWriter, metadata map[string]any) {
	if completed, _ := metadata[streamCompletedKey].(bool); completed {
		_ = writer.Close()
		return
	}
	if _, err := writer.Write(streamFailureEvent("provider stream failed")); err != nil {
		_ = writer.CloseWithError(err)
		return
	}
	_ = writer.Close()
}

func streamFailureEvent(message string) []byte {
	return responseEvent("response.failed", map[string]any{
		"response": map[string]any{
			"error": map[string]any{"code": "provider_stream_error", "message": message},
		},
	})
}

func writeTranslatedEvents(writer *io.PipeWriter, events [][]byte, providerKey string, responseMetadata, metadata map[string]any) error {
	for _, event := range events {
		translated, supported, err := translateChatSSEData(event, providerKey, responseMetadata, metadata)
		if err != nil {
			return err
		}
		if supported {
			if _, err := writer.Write(translated); err != nil {
				return err
			}
		}
	}
	return nil
}

func TranslateChatSSEData(data []byte) ([]byte, bool, error) {
	return translateChatSSEData(data, "", nil, nil)
}

func translateChatSSEData(data []byte, providerKey string, responseMetadata, metadata map[string]any) ([]byte, bool, error) {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if string(data) == "[DONE]" {
		return completedEvent(providerKey, responseMetadata, metadata)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, false, errors.New("failed to parse chat completions SSE JSON")
	}
	choice := firstStreamChoice(root)
	if choice == nil {
		return nil, false, nil
	}
	delta, _ := choice["delta"].(map[string]any)
	if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
		appendReasoningMetadata(metadata, reasoning)
	}
	if finish, ok := choice["finish_reason"].(string); ok && finish != "" {
		metadata["finish_reason"] = finish
	}
	if tool := firstToolDelta(delta); tool != nil {
		function, _ := tool["function"].(map[string]any)
		arguments, ok := function["arguments"].(string)
		if ok {
			name, _ := function["name"].(string)
			arguments = WrapRTKArguments(name, arguments)
			payload := map[string]any{"type": "response.function_call_arguments.delta", "delta": arguments}
			if id, ok := tool["id"].(string); ok && id != "" {
				payload["call_id"] = id
			}
			return responseEvent("response.function_call_arguments.delta", payload), true, nil
		}
	}
	if text, ok := delta["content"].(string); ok && text != "" {
		return responseEvent("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "delta": text}), true, nil
	}
	if finish, ok := choice["finish_reason"].(string); ok && finish != "" {
		return completedEvent(providerKey, responseMetadata, metadata)
	}
	return nil, false, nil
}

func completedEvent(providerKey string, responseMetadata, metadata map[string]any) ([]byte, bool, error) {
	if completed, _ := metadata[streamCompletedKey].(bool); completed {
		return nil, false, nil
	}
	metadata[streamCompletedKey] = true
	return responseEvent("response.completed", completedPayload(providerKey, responseMetadata, metadata)), true, nil
}

func completedPayload(providerKey string, responseMetadata, metadata map[string]any) map[string]any {
	payload := map[string]any{}
	if providerKey == "" {
		return payload
	}
	result := make(map[string]any, len(responseMetadata)+1)
	for key, value := range responseMetadata {
		result[key] = value
	}
	merged := make(map[string]any)
	if provider, ok := responseMetadata[providerKey].(map[string]any); ok {
		for key, value := range provider {
			merged[key] = value
		}
	}
	for key, value := range metadata {
		if key != streamReasoningBuilderKey && key != streamCompletedKey {
			merged[key] = value
		}
	}
	if reasoning, ok := metadata[streamReasoningBuilderKey].(*strings.Builder); ok && reasoning.Len() > 0 {
		merged["reasoning_content"] = reasoning.String()
	}
	if len(merged) > 0 || len(result) > 0 {
		result[providerKey] = merged
		payload["metadata"] = result
	}
	return payload
}

func appendReasoningMetadata(metadata map[string]any, value string) {
	builder, ok := metadata[streamReasoningBuilderKey].(*strings.Builder)
	if !ok {
		builder = &strings.Builder{}
		metadata[streamReasoningBuilderKey] = builder
	}
	remaining := streamMetadataMaxBytes - builder.Len()
	if remaining <= 0 {
		return
	}
	if len(value) > remaining {
		value = strings.ToValidUTF8(value[:remaining], "")
	}
	_, _ = builder.WriteString(value)
}

func firstStreamChoice(root map[string]any) map[string]any {
	choices, ok := root["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	return choice
}

func firstToolDelta(delta map[string]any) map[string]any {
	if delta == nil {
		return nil
	}
	calls, ok := delta["tool_calls"].([]any)
	if !ok || len(calls) == 0 {
		return nil
	}
	call, _ := calls[0].(map[string]any)
	return call
}

func responseEvent(name string, payload map[string]any) []byte {
	payload["type"] = strings.TrimSpace(name)
	content, err := json.Marshal(payload)
	if err != nil {
		return []byte(fmt.Sprintf("event: %s\ndata: {}\n\n", name))
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", name, content))
}
