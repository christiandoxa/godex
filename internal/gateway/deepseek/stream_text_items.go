package deepseek

import "fmt"

func (state *deepSeekChatStreamState) outputTextItemID() string {
	return fmt.Sprintf("msg_deepseek_%d", state.requestID)
}

func (state *deepSeekChatStreamState) outputTextItemAddedEvent() []byte {
	return anthropicStreamEvent("response.output_item.added", map[string]any{
		"type": "response.output_item.added", "sequence_number": state.nextSequenceNumber(),
		"response_id": state.responseID,
		"item": map[string]any{
			"id": state.outputTextItemID(), "type": "message", "role": "assistant", "content": []any{},
		},
	})
}

func (state *deepSeekChatStreamState) outputTextItemDoneEvent() []byte {
	return anthropicStreamEvent("response.output_item.done", map[string]any{
		"type": "response.output_item.done", "sequence_number": state.nextSequenceNumber(),
		"response_id": state.responseID,
		"item": map[string]any{
			"id": state.outputTextItemID(), "type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": state.outputText.String()}},
		},
	})
}
