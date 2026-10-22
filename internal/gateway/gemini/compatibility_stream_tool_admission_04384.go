package gemini

import "io"

func (state *geminiChatCompatibleStream043711) observeToolAdmission(writer io.Writer, chunk map[string]any) error {
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	calls, _ := delta["tool_calls"].([]any)
	for _, value := range calls {
		call, _ := value.(map[string]any)
		index := nativeUint(call["index"])
		if uint64(uint(index)) != index {
			index = 0
		}
		if _, known := state.toolIndices[index]; known {
			continue
		}
		if _, object := call["function"].(map[string]any); !object {
			if err := state.failure(writer, "invalid_tool_call_arguments", "Gemini OpenAI-compatible streamed a tool call without a function object"); err != nil {
				return err
			}
			continue
		}
		// shortcut: bound retained index ownership; qualify larger tool streams before raising this budget.
		if len(state.toolIndices) >= geminiStreamMetadataMaxBytes04384/128 {
			state.eof = true
			return state.failure(writer, "provider_stream_error", "provider tool state exceeded safe size limit (67108864)")
		}
		if state.toolIndices == nil {
			state.toolIndices = make(map[uint64]struct{})
		}
		state.toolIndices[index] = struct{}{}
	}
	return nil
}
