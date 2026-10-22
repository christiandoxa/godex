package gemini

import "io"

func (state *geminiChatCompatibleStream043711) observeChoiceOutput(writer io.Writer, chunk map[string]any) error {
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	if text, _ := delta["content"].(string); text != "" {
		if err := state.emitText(writer, text); err != nil || state.eof {
			return err
		}
	}
	calls, _ := delta["tool_calls"].([]any)
	for _, value := range calls {
		call, _ := value.(map[string]any)
		if err := state.observeToolDelta(writer, call); err != nil || state.eof {
			return err
		}
	}
	if _, present := choice["finish_reason"].(string); present {
		if valid, err := state.completeToolEvents(writer); err != nil || !valid {
			return err
		}
		if len(state.toolCalls) > 0 {
			state.rememberConversation()
		}
	}
	return nil
}

func (state *geminiChatCompatibleStream043711) emitText(writer io.Writer, text string) error {
	// shortcut: completion text is capped at 64 MiB; qualify larger streams before raising this budget.
	if len(text) > geminiStreamMetadataMaxBytes04384-state.text.Len() {
		state.eof = true
		return state.failure(writer, "provider_stream_error", "provider text exceeded safe size limit (67108864)")
	}
	if err := state.openTextItem(writer); err != nil {
		return err
	}
	state.text.WriteString(text)
	return state.send(writer, "response.output_text.delta", map[string]any{
		"created_at": state.created, "response_id": state.id, "delta": text,
	})
}
