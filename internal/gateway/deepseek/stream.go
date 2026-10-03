package deepseek

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func deepSeekChatSSE(body io.ReadCloser) io.ReadCloser {
	reader, writer := io.Pipe()
	go pumpDeepSeekChatSSE(body, writer)
	return reader
}

func pumpDeepSeekChatSSE(body io.ReadCloser, writer *io.PipeWriter) {
	defer body.Close()
	decoder := sse.NewDecoder(streamEventMaxBytes)
	buffer := make([]byte, 32<<10)
	for {
		read, err := body.Read(buffer)
		if read > 0 {
			for _, event := range decoder.Feed(buffer[:read]) {
				translated, supported, translateErr := translateDeepSeekSSEData(event)
				if translateErr != nil {
					_ = writer.CloseWithError(translateErr)
					return
				}
				if supported {
					if _, writeErr := writer.Write(translated); writeErr != nil {
						_ = writer.CloseWithError(writeErr)
						return
					}
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = writer.Close()
			} else {
				_ = writer.CloseWithError(err)
			}
			return
		}
	}
}

func translateDeepSeekSSEData(data []byte) ([]byte, bool, error) {
	if string(data) == "[DONE]" {
		return []byte("event: response.completed\ndata: {}\n\n"), true, nil
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil, false, errors.New("failed to parse DeepSeek SSE JSON")
	}
	choices, ok := root["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil, false, nil
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	rawDelta, ok := choice["delta"]
	if !ok {
		return nil, false, nil
	}
	delta, _ := rawDelta.(map[string]any)
	if calls, ok := delta["tool_calls"].([]any); ok && len(calls) > 0 {
		call, ok := calls[0].(map[string]any)
		if !ok {
			return nil, false, nil
		}
		function, ok := call["function"].(map[string]any)
		if !ok {
			return nil, false, nil
		}
		arguments, ok := function["arguments"].(string)
		if !ok {
			return nil, false, nil
		}
		payload := map[string]any{"delta": arguments, "type": "response.function_call_arguments.delta"}
		if id, ok := call["id"].(string); ok {
			payload["call_id"] = id
		}
		return deepSeekStreamEvent("response.function_call_arguments.delta", payload), true, nil
	}
	// DeepSeek emits empty text deltas for present deltas without text or tool arguments.
	text, _ := delta["content"].(string)
	return deepSeekStreamEvent("response.output_text.delta", map[string]any{
		"delta": text,
		"type":  "response.output_text.delta",
	}), true, nil
}

func deepSeekStreamEvent(name string, payload map[string]any) []byte {
	content, _ := json.Marshal(payload)
	return []byte("event: " + name + "\ndata: " + string(content) + "\n\n")
}
