package gemini

import "io"

func (state *geminiChatCompatibleStream043711) observeReasoning(writer io.Writer, chunk map[string]any) error {
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	text, _ := delta["reasoning_content"].(string)
	if text == "" {
		return nil
	}
	if len(text) > geminiStreamMetadataMaxBytes04384-state.reasoning.Len() {
		return state.failure(writer, "provider_stream_error", "provider reasoning exceeded safe size limit (67108864)")
	}
	if state.reasoning.Len() == 0 {
		if err := state.send(writer, "response.reasoning_summary_part.added", map[string]any{
			"response_id": state.id, "output_index": 0, "summary_index": 0,
			"part": map[string]any{"type": "summary_text", "text": ""},
		}); err != nil {
			return err
		}
	}
	state.reasoning.WriteString(text)
	return state.send(writer, "response.reasoning_summary_text.delta", map[string]any{
		"response_id": state.id, "output_index": 0, "summary_index": 0, "delta": text,
	})
}

func (state *geminiChatCompatibleStream043711) completeReasoning(writer io.Writer) error {
	if state.reasoning.Len() == 0 {
		return nil
	}
	return state.send(writer, "response.reasoning_summary_text.done", map[string]any{
		"response_id": state.id, "output_index": 0, "summary_index": 0, "text": state.reasoning.String(),
	})
}
