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

func ChatSSE(body io.ReadCloser) io.ReadCloser {
	reader, writer := io.Pipe()
	go pumpChatSSE(body, writer)
	return reader
}

func pumpChatSSE(body io.ReadCloser, writer *io.PipeWriter) {
	defer body.Close()
	decoder := sse.NewDecoder(streamEventMaxBytes)
	buffer := make([]byte, 32<<10)
	for {
		read, err := body.Read(buffer)
		if read > 0 {
			if writeErr := writeTranslatedEvents(writer, decoder.Feed(buffer[:read])); writeErr != nil {
				_ = writer.CloseWithError(writeErr)
				return
			}
		}
		if err != nil {
			closeChatSSEWriter(writer, err)
			return
		}
	}
}

func writeTranslatedEvents(writer *io.PipeWriter, events [][]byte) error {
	for _, event := range events {
		translated, supported, err := TranslateChatSSEData(event)
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

func closeChatSSEWriter(writer *io.PipeWriter, err error) {
	if errors.Is(err, io.EOF) {
		_ = writer.Close()
		return
	}
	_ = writer.CloseWithError(err)
}

func TranslateChatSSEData(data []byte) ([]byte, bool, error) {
	if string(data) == "[DONE]" {
		return responseEvent("response.completed", map[string]any{}), true, nil
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
	if tool := firstToolDelta(delta); tool != nil {
		function, _ := tool["function"].(map[string]any)
		arguments, ok := function["arguments"].(string)
		if ok {
			name, _ := function["name"].(string)
			arguments = wrapRTKArguments(name, arguments)
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
		return responseEvent("response.completed", map[string]any{}), true, nil
	}
	return nil, false, nil
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
